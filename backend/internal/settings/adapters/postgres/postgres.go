// Package postgres implements the settings repository on a pgx pool.
//
// Money and rates are NUMERIC columns. They cross the driver as text
// (`$n::text::numeric` on the way in, `col::text` on the way out) so no
// precision is lost through a float.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Repo stores the settings in the settings tables.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func dec(s string) (decimal.Decimal, error) { return decimal.NewFromString(s) }

func decPtr(s *string) (*decimal.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	d, err := dec(*s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func strPtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// inTx runs fn in a transaction that is rolled back on error.
func (r *Repo) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type allocationJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// IsEmpty reports whether no settings row exists.
func (r *Repo) IsEmpty(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM settings)`).Scan(&exists)
	return !exists, err
}

// Load reads the whole configuration. It returns an empty Config when the
// settings row does not exist yet.
func (r *Repo) Load(ctx context.Context) (domain.Config, error) {
	var cfg domain.Config
	// One snapshot so the sections are consistent with each other.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return cfg, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, load := range []func(context.Context, pgx.Tx, *domain.Config) error{
		loadGeneral, loadCategories, loadClients, loadInstruments,
		loadBrackets, loadPaymentMethods, loadIssuer, loadPause,
	} {
		if err := load(ctx, tx, &cfg); err != nil {
			return domain.Config{}, err
		}
	}
	return cfg, nil
}

func loadGeneral(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	var salary, fx, fee, months, extra string
	var cycleStartDay int16
	var split, alloc []byte
	err := tx.QueryRow(ctx,
		`SELECT salary_usd::text, fx_rate_applied::text, morse_fee_rate::text, emergency_months::text,
		        extra_income_estimate_mxn::text, budget_includes_extra_income, cycle_start_day, extra_income_split, investment_allocation
		 FROM settings WHERE id = 1`).
		Scan(&salary, &fx, &fee, &months, &extra, &cfg.BudgetIncludesExtraIncome, &cycleStartDay, &split, &alloc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range []struct {
		dst **decimal.Decimal
		src string
	}{{&cfg.SalaryUSD, salary}, {&cfg.FXRateApplied, fx}, {&cfg.MorseFeeRate, fee}, {&cfg.EmergencyMonths, months}} {
		d, err := dec(f.src)
		if err != nil {
			return err
		}
		*f.dst = &d
	}
	if cfg.ExtraIncomeEstimateMXN, err = dec(extra); err != nil {
		return err
	}
	cfg.CycleStartDay = int(cycleStartDay)

	var splitRaw map[string]string
	if err := json.Unmarshal(split, &splitRaw); err != nil {
		return fmt.Errorf("decode extra_income_split: %w", err)
	}
	cfg.ExtraIncomeSplit = make(map[string]decimal.Decimal, len(splitRaw))
	for k, v := range splitRaw {
		if cfg.ExtraIncomeSplit[k], err = dec(v); err != nil {
			return err
		}
	}
	var allocRaw []allocationJSON
	if err := json.Unmarshal(alloc, &allocRaw); err != nil {
		return fmt.Errorf("decode investment_allocation: %w", err)
	}
	for _, a := range allocRaw {
		v, err := dec(a.Value)
		if err != nil {
			return err
		}
		cfg.InvestmentAllocation = append(cfg.InvestmentAllocation, domain.Weight{Key: a.Key, Value: v})
	}
	return nil
}

func loadCategories(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	rows, err := tx.Query(ctx, `SELECT name, kind, budget::text, includes, keywords FROM categories ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Category
		var kind string
		var budget *string
		if err := rows.Scan(&c.Name, &kind, &budget, &c.Includes, &c.Keywords); err != nil {
			return err
		}
		c.Kind = ledger.Kind(kind)
		if c.Budget, err = decPtr(budget); err != nil {
			return err
		}
		cfg.Categories = append(cfg.Categories, c)
	}
	return rows.Err()
}

func loadClients(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	rows, err := tx.Query(ctx,
		`SELECT id, name, currency, iva_rate::text, type, rfc, regimen, uso_cfdi, ret_isr_rate::text, ret_iva_rate::text,
		        concepto, clave_prod_serv, clave_unidad, address, tax_residence, contract, real_payer
		 FROM clients ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Client
		var iva, retISR, retIVA string
		if err := rows.Scan(&c.ID, &c.Name, &c.Currency, &iva, &c.Type, &c.RFC, &c.Regimen, &c.UsoCFDI, &retISR, &retIVA,
			&c.Concepto, &c.ClaveProdServ, &c.ClaveUnidad, &c.Address, &c.TaxResidence, &c.Contract, &c.RealPayer); err != nil {
			return err
		}
		for _, f := range []struct {
			dst *decimal.Decimal
			src string
		}{{&c.IVARate, iva}, {&c.RetISRRate, retISR}, {&c.RetIVARate, retIVA}} {
			if *f.dst, err = dec(f.src); err != nil {
				return err
			}
		}
		cfg.Clients = append(cfg.Clients, c)
	}
	return rows.Err()
}

func loadInstruments(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	rows, err := tx.Query(ctx, `SELECT id, name, type, platform FROM instruments ORDER BY position`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var i domain.Instrument
		if err := rows.Scan(&i.ID, &i.Name, &i.Type, &i.Platform); err != nil {
			rows.Close()
			return err
		}
		cfg.Instruments = append(cfg.Instruments, i)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = tx.Query(ctx, `SELECT category_name, instrument_id FROM instrument_by_category`)
	if err != nil {
		return err
	}
	defer rows.Close()
	cfg.InstrumentByCategory = map[string]string{}
	for rows.Next() {
		var cat, id string
		if err := rows.Scan(&cat, &id); err != nil {
			return err
		}
		cfg.InstrumentByCategory[cat] = id
	}
	return rows.Err()
}

func loadBrackets(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	rows, err := tx.Query(ctx, `SELECT upper::text, rate::text FROM resico_brackets ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var upper, rate string
		if err := rows.Scan(&upper, &rate); err != nil {
			return err
		}
		var b domain.Bracket
		if b.Upper, err = dec(upper); err != nil {
			return err
		}
		if b.Rate, err = dec(rate); err != nil {
			return err
		}
		cfg.Brackets = append(cfg.Brackets, b)
	}
	return rows.Err()
}

func loadPaymentMethods(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	rows, err := tx.Query(ctx, `SELECT name FROM payment_methods ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		cfg.PaymentMethods = append(cfg.PaymentMethods, name)
	}
	return rows.Err()
}

func loadIssuer(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	err := tx.QueryRow(ctx, `SELECT rfc, name, regimen, postal_code, note FROM issuer WHERE id = 1`).
		Scan(&cfg.Issuer.RFC, &cfg.Issuer.Name, &cfg.Issuer.Regimen, &cfg.Issuer.PostalCode, &cfg.Issuer.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func loadPause(ctx context.Context, tx pgx.Tx, cfg *domain.Config) error {
	var p domain.PausePlan
	var normal string
	err := tx.QueryRow(ctx, `SELECT normal_budget::text, resume_month, note FROM investment_pause WHERE id = 1`).
		Scan(&normal, &p.ResumeMonth, &p.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.NormalBudget, err = dec(normal); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT month, plan_amount::text FROM investment_pause_plan ORDER BY position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	p.FutureExpensesPlan = map[string]decimal.Decimal{}
	for rows.Next() {
		var month, amount string
		if err := rows.Scan(&month, &amount); err != nil {
			return err
		}
		d, err := dec(amount)
		if err != nil {
			return err
		}
		p.Months = append(p.Months, month)
		p.FutureExpensesPlan[month] = d
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cfg.Pause = &p
	return nil
}

// SaveGeneral upserts the singleton settings row.
func (r *Repo) SaveGeneral(ctx context.Context, g domain.General) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveGeneral(ctx, tx, g) })
}

func saveGeneral(ctx context.Context, tx pgx.Tx, g domain.General) error {
	split := make(map[string]string, len(g.ExtraIncomeSplit))
	for k, v := range g.ExtraIncomeSplit {
		split[k] = v.String()
	}
	alloc := make([]allocationJSON, len(g.InvestmentAllocation))
	for i, w := range g.InvestmentAllocation {
		alloc[i] = allocationJSON{Key: w.Key, Value: w.Value.String()}
	}
	splitJSON, err := json.Marshal(split)
	if err != nil {
		return err
	}
	allocJSON, err := json.Marshal(alloc)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO settings (id, salary_usd, fx_rate_applied, morse_fee_rate, emergency_months,
		                       extra_income_estimate_mxn, budget_includes_extra_income, cycle_start_day,
		                       extra_income_split, investment_allocation)
		 VALUES (1, $1::text::numeric, $2::text::numeric, $3::text::numeric, $4::text::numeric, $5::text::numeric, $6, $7::smallint, $8, $9)
		 ON CONFLICT (id) DO UPDATE SET
		   salary_usd = EXCLUDED.salary_usd, fx_rate_applied = EXCLUDED.fx_rate_applied,
		   morse_fee_rate = EXCLUDED.morse_fee_rate, emergency_months = EXCLUDED.emergency_months,
		   extra_income_estimate_mxn = EXCLUDED.extra_income_estimate_mxn,
		   budget_includes_extra_income = EXCLUDED.budget_includes_extra_income,
		   cycle_start_day = EXCLUDED.cycle_start_day,
		   extra_income_split = EXCLUDED.extra_income_split,
		   investment_allocation = EXCLUDED.investment_allocation, updated_at = now()`,
		g.SalaryUSD.String(), g.FXRateApplied.String(), g.MorseFeeRate.String(), g.EmergencyMonths.String(),
		g.ExtraIncomeEstimateMXN.String(), g.BudgetIncludesExtraIncome, int16(g.CycleStartDay), splitJSON, allocJSON)
	return err
}

// SaveCategories replaces every category.
func (r *Repo) SaveCategories(ctx context.Context, cats []domain.Category) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveCategories(ctx, tx, cats) })
}

func saveCategories(ctx context.Context, tx pgx.Tx, cats []domain.Category) error {
	if _, err := tx.Exec(ctx, `DELETE FROM categories`); err != nil {
		return err
	}
	for i, c := range cats {
		kw := c.Keywords
		if kw == nil {
			kw = []string{}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO categories (name, position, kind, budget, includes, keywords)
			 VALUES ($1, $2, $3, $4::text::numeric, $5, $6)`,
			c.Name, i, string(c.Kind), strPtr(c.Budget), c.Includes, kw); err != nil {
			return fmt.Errorf("insert category %q: %w", c.Name, err)
		}
	}
	return nil
}

// SaveClients replaces every client.
func (r *Repo) SaveClients(ctx context.Context, clients []domain.Client) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveClients(ctx, tx, clients) })
}

func saveClients(ctx context.Context, tx pgx.Tx, clients []domain.Client) error {
	if _, err := tx.Exec(ctx, `DELETE FROM clients`); err != nil {
		return err
	}
	for i, c := range clients {
		if _, err := tx.Exec(ctx,
			`INSERT INTO clients (id, position, name, type, currency, iva_rate, rfc, regimen, uso_cfdi,
			                      ret_isr_rate, ret_iva_rate, concepto, clave_prod_serv, clave_unidad,
			                      address, tax_residence, contract, real_payer)
			 VALUES ($1, $2, $3, $4, $5, $6::text::numeric, $7, $8, $9, $10::text::numeric, $11::text::numeric,
			         $12, $13, $14, $15, $16, $17, $18)`,
			c.ID, i, c.Name, c.Type, c.Currency, c.IVARate.String(), c.RFC, c.Regimen, c.UsoCFDI,
			c.RetISRRate.String(), c.RetIVARate.String(), c.Concepto, c.ClaveProdServ, c.ClaveUnidad,
			c.Address, c.TaxResidence, c.Contract, c.RealPayer); err != nil {
			return fmt.Errorf("insert client %q: %w", c.ID, err)
		}
	}
	return nil
}

// SaveInstruments replaces the instruments and the per-category defaults.
func (r *Repo) SaveInstruments(ctx context.Context, instruments []domain.Instrument, byCategory map[string]string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveInstruments(ctx, tx, instruments, byCategory) })
}

func saveInstruments(ctx context.Context, tx pgx.Tx, instruments []domain.Instrument, byCategory map[string]string) error {
	// The mapping references the instruments, so it goes first on delete and last on insert.
	if _, err := tx.Exec(ctx, `DELETE FROM instrument_by_category`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM instruments`); err != nil {
		return err
	}
	for i, in := range instruments {
		if _, err := tx.Exec(ctx,
			`INSERT INTO instruments (id, position, name, type, platform) VALUES ($1, $2, $3, $4, $5)`,
			in.ID, i, in.Name, in.Type, in.Platform); err != nil {
			return fmt.Errorf("insert instrument %q: %w", in.ID, err)
		}
	}
	for cat, id := range byCategory {
		if _, err := tx.Exec(ctx,
			`INSERT INTO instrument_by_category (category_name, instrument_id) VALUES ($1, $2)`, cat, id); err != nil {
			return fmt.Errorf("insert default instrument of %q: %w", cat, err)
		}
	}
	return nil
}

// SaveBrackets replaces the RESICO brackets.
func (r *Repo) SaveBrackets(ctx context.Context, brackets []domain.Bracket) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveBrackets(ctx, tx, brackets) })
}

func saveBrackets(ctx context.Context, tx pgx.Tx, brackets []domain.Bracket) error {
	if _, err := tx.Exec(ctx, `DELETE FROM resico_brackets`); err != nil {
		return err
	}
	for i, b := range brackets {
		if _, err := tx.Exec(ctx,
			`INSERT INTO resico_brackets (position, upper, rate) VALUES ($1, $2::text::numeric, $3::text::numeric)`,
			i, b.Upper.String(), b.Rate.String()); err != nil {
			return fmt.Errorf("insert bracket %d: %w", i+1, err)
		}
	}
	return nil
}

// SavePaymentMethods replaces the payment methods.
func (r *Repo) SavePaymentMethods(ctx context.Context, methods []string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return savePaymentMethods(ctx, tx, methods) })
}

func savePaymentMethods(ctx context.Context, tx pgx.Tx, methods []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM payment_methods`); err != nil {
		return err
	}
	for i, m := range methods {
		if _, err := tx.Exec(ctx, `INSERT INTO payment_methods (name, position) VALUES ($1, $2)`, m, i); err != nil {
			return fmt.Errorf("insert payment method %q: %w", m, err)
		}
	}
	return nil
}

// SaveIssuer upserts the singleton issuer row.
func (r *Repo) SaveIssuer(ctx context.Context, issuer domain.Issuer) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return saveIssuer(ctx, tx, issuer) })
}

func saveIssuer(ctx context.Context, tx pgx.Tx, i domain.Issuer) error {
	if i.IsZero() {
		_, err := tx.Exec(ctx, `DELETE FROM issuer`)
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO issuer (id, rfc, name, regimen, postal_code, note) VALUES (1, $1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET rfc = EXCLUDED.rfc, name = EXCLUDED.name, regimen = EXCLUDED.regimen,
		   postal_code = EXCLUDED.postal_code, note = EXCLUDED.note`,
		i.RFC, i.Name, i.Regimen, i.PostalCode, i.Note)
	return err
}

// SavePause replaces the pause plan; nil removes it.
func (r *Repo) SavePause(ctx context.Context, plan *domain.PausePlan) error {
	return r.inTx(ctx, func(tx pgx.Tx) error { return savePause(ctx, tx, plan) })
}

func savePause(ctx context.Context, tx pgx.Tx, plan *domain.PausePlan) error {
	if _, err := tx.Exec(ctx, `DELETE FROM investment_pause_plan`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM investment_pause`); err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO investment_pause (id, normal_budget, resume_month, note) VALUES (1, $1::text::numeric, $2, $3)`,
		plan.NormalBudget.String(), plan.ResumeMonth, plan.Note); err != nil {
		return err
	}
	for i, m := range plan.Months {
		if _, err := tx.Exec(ctx,
			`INSERT INTO investment_pause_plan (month, position, plan_amount) VALUES ($1, $2, $3::text::numeric)`,
			m, i, plan.FutureExpensesPlan[m].String()); err != nil {
			return fmt.Errorf("insert pause month %s: %w", m, err)
		}
	}
	return nil
}

// Import replaces every section in one transaction.
func (r *Repo) Import(ctx context.Context, cfg domain.Config) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		if err := saveGeneral(ctx, tx, cfg.General()); err != nil {
			return err
		}
		if err := saveCategories(ctx, tx, cfg.Categories); err != nil {
			return err
		}
		if err := saveClients(ctx, tx, cfg.Clients); err != nil {
			return err
		}
		if err := saveInstruments(ctx, tx, cfg.Instruments, cfg.InstrumentByCategory); err != nil {
			return err
		}
		if err := saveBrackets(ctx, tx, cfg.Brackets); err != nil {
			return err
		}
		if err := savePaymentMethods(ctx, tx, cfg.PaymentMethods); err != nil {
			return err
		}
		if err := saveIssuer(ctx, tx, cfg.Issuer); err != nil {
			return err
		}
		return savePause(ctx, tx, cfg.Pause)
	})
}
