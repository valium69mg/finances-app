import { describe, expect, it } from "vitest";
import { parseCfdi, prettyXml } from "./cfdi";

const UUID = "6F1B2C3D-4E5F-4A6B-8C7D-9E0F1A2B3C4D";

const V40 = `<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="4.0" Serie="A" Folio="12" Fecha="2026-10-02T10:00:00" SubTotal="1000.00" Moneda="USD" Total="1000.00">
  <cfdi:Emisor Rfc="EMI010101AAA" Nombre="Emisor SA" RegimenFiscal="626"/>
  <cfdi:Receptor Rfc="REC020202BBB" Nombre="Cliente LLC"/>
  <cfdi:Complemento><tfd:TimbreFiscalDigital Version="1.1" UUID="${UUID.toLowerCase()}"/></cfdi:Complemento>
</cfdi:Comprobante>`;

const V33 = `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/3" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="3.3" Fecha="2021-05-01T09:00:00" SubTotal="500" Moneda="MXN" Total="580">
  <cfdi:Emisor Rfc="EMI010101AAA" Nombre="Emisor SA"/>
  <cfdi:Receptor Rfc="REC020202BBB"/>
  <cfdi:Complemento><tfd:TimbreFiscalDigital UUID="${UUID}"/></cfdi:Complemento>
</cfdi:Comprobante>`;

describe("parseCfdi", () => {
  it("reads a CFDI 4.0", () => {
    expect(parseCfdi(V40)).toEqual({
      emisorNombre: "Emisor SA",
      emisorRfc: "EMI010101AAA",
      receptorNombre: "Cliente LLC",
      receptorRfc: "REC020202BBB",
      fecha: "2026-10-02T10:00:00",
      serie: "A",
      folio: "12",
      subTotal: "1000.00",
      total: "1000.00",
      moneda: "USD",
      uuid: UUID,
    });
  });

  it("reads a CFDI 3.3 and leaves missing fields undefined", () => {
    const s = parseCfdi(V33);
    expect(s).toMatchObject({ emisorRfc: "EMI010101AAA", receptorRfc: "REC020202BBB", total: "580", moneda: "MXN", uuid: UUID });
    expect(s?.receptorNombre).toBeUndefined();
    expect(s?.serie).toBeUndefined();
    expect(s?.folio).toBeUndefined();
  });

  it("does not throw when the stamp or parties are missing", () => {
    const s = parseCfdi(`<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="1"/>`);
    expect(s?.total).toBe("1");
    expect(s?.uuid).toBeUndefined();
    expect(s?.emisorNombre).toBeUndefined();
  });

  it("returns null for invalid XML, a non-CFDI root or plain text", () => {
    expect(parseCfdi("<cfdi:Comprobante")).toBeNull();
    expect(parseCfdi("<root><a/></root>")).toBeNull();
    expect(parseCfdi("hello")).toBeNull();
    expect(parseCfdi("")).toBeNull();
  });
});

describe("prettyXml", () => {
  it("indents nested elements and keeps attributes", () => {
    const out = prettyXml(V33);
    expect(out).toContain('\n  <cfdi:Emisor Rfc="EMI010101AAA" Nombre="Emisor SA"/>');
    expect(out).toContain(`\n    <tfd:TimbreFiscalDigital UUID="${UUID}"/>`);
  });

  it("returns invalid input untouched", () => {
    expect(prettyXml("<a")).toBe("<a");
  });
});
