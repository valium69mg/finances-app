import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

afterEach(cleanup);

describe("Modal", () => {
  it("closes with Escape even when the focus is on the page body", () => {
    const onClose = vi.fn();
    render(
      <Modal title="Uno" onClose={onClose}>
        <button type="button">dentro</button>
      </Modal>,
    );
    (document.activeElement as HTMLElement | null)?.blur();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes only the innermost of two stacked modals", () => {
    function Stack() {
      const [inner, setInner] = useState(true);
      const [outer, setOuter] = useState(true);
      return (
        <>
          {outer && (
            <Modal title="Externo" onClose={() => setOuter(false)}>
              <p>lista</p>
            </Modal>
          )}
          {inner && (
            <Modal title="Interno" onClose={() => setInner(false)}>
              <p>formulario</p>
            </Modal>
          )}
        </>
      );
    }
    render(<Stack />);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Interno" })).toBeNull();
    expect(screen.getByRole("dialog", { name: "Externo" })).toBeInTheDocument();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("gives the focus back to the element that opened it", () => {
    function Host() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            abrir
          </button>
          {open && (
            <Modal title="Uno" onClose={() => setOpen(false)}>
              <input aria-label="campo" />
            </Modal>
          )}
        </>
      );
    }
    render(<Host />);
    const trigger = screen.getByRole("button", { name: "abrir" });
    trigger.focus();
    fireEvent.click(trigger);
    expect(screen.getByRole("textbox", { name: "campo" })).toHaveFocus();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(trigger).toHaveFocus();
  });
});
