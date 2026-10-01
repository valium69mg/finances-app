import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { InlineFormPanel } from "./InlineFormPanel";

afterEach(cleanup);

function Harness({ startEditing = false }: { startEditing?: boolean }) {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState(startEditing);
  return (
    <div>
      <button type="button" onClick={() => setEditing(true)}>
        editar
      </button>
      <InlineFormPanel id="panel" label="Nuevo ingreso" open={open} onToggle={() => setOpen((o) => !o)} editing={editing}>
        <form aria-label="form">
          <input aria-label="Descripción" />
          <button
            type="button"
            onClick={() => {
              setEditing(false);
              setOpen(false);
            }}
          >
            guardar
          </button>
        </form>
      </InlineFormPanel>
    </div>
  );
}

describe("InlineFormPanel", () => {
  it("is collapsed by default, without rendering the form", () => {
    render(<Harness />);
    const toggle = screen.getByRole("button", { name: "Nuevo ingreso" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(toggle).toHaveAttribute("aria-controls", "panel");
    expect(screen.queryByRole("form")).not.toBeInTheDocument();
  });

  it("expands inline, moves focus to the first field and toggles closed again", () => {
    render(<Harness />);
    const toggle = screen.getByRole("button", { name: "Nuevo ingreso" });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("textbox", { name: "Descripción" })).toHaveFocus();

    fireEvent.click(toggle);
    expect(screen.queryByRole("form")).not.toBeInTheDocument();
    expect(toggle).toHaveFocus();
  });

  it("opens by itself while editing, hides the toggle and gives the focus back after saving", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "editar" }));
    expect(screen.getByRole("form")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Nuevo ingreso" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "guardar" }));
    expect(screen.queryByRole("form")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nuevo ingreso" })).toHaveFocus();
  });
});
