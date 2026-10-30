import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

afterEach(cleanup);

describe("Modal", () => {
  it("focuses the first field without scrolling and starts both scroll containers at the top", () => {
    const focus = vi.spyOn(HTMLElement.prototype, "focus");
    render(
      <Modal title="Largo" onClose={() => {}}>
        <input aria-label="campo" />
      </Modal>,
    );
    const field = screen.getByLabelText("campo");
    expect(focus.mock.instances).toContain(field);
    expect(focus.mock.calls[focus.mock.instances.indexOf(field)][0]).toEqual({ preventScroll: true });
    expect(field.closest("div.overflow-y-auto")!.scrollTop).toBe(0);
    focus.mockRestore();
  });

  it("starts on the element marked data-autofocus", () => {
    render(
      <Modal title="Confirmar" onClose={() => {}}>
        <input aria-label="campo" />
        <button type="button" data-autofocus>
          Cancelar
        </button>
      </Modal>,
    );
    expect(screen.getByRole("button", { name: "Cancelar" })).toHaveFocus();
  });

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

  it("as a sheet it is anchored at the bottom on every width and shows a drag handle", () => {
    render(
      <Modal title="Más" onClose={() => {}} sheet>
        <button type="button">dentro</button>
      </Modal>,
    );
    const dialog = screen.getByRole("dialog", { name: "Más" });
    expect(dialog.className).not.toContain("sm:items-center");
    expect(dialog.querySelector(".rounded-full.bg-border")).not.toBeNull();
  });

  it("without sheet it keeps the centered layout and no handle", () => {
    render(
      <Modal title="Uno" onClose={() => {}}>
        <button type="button">dentro</button>
      </Modal>,
    );
    const dialog = screen.getByRole("dialog", { name: "Uno" });
    expect(dialog.className).toContain("sm:items-center");
    expect(dialog.querySelector(".h-1\\.5")).toBeNull();
  });
});
