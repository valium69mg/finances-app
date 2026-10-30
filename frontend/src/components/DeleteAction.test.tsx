import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DeleteAction } from "./DeleteAction";

afterEach(cleanup);

describe("DeleteAction", () => {
  it("asks before deleting and only confirms on the second step", () => {
    const onConfirm = vi.fn();
    render(<DeleteAction name="Laptop" question="¿Eliminar este gasto futuro?" pending={false} onConfirm={onConfirm} />);
    fireEvent.click(screen.getByRole("button", { name: "Eliminar Laptop" }));
    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByRole("group", { name: "Confirmar eliminación de Laptop" })).toHaveTextContent("¿Eliminar este gasto futuro?");
    fireEvent.click(screen.getByRole("button", { name: "Sí, eliminar" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("moves the focus to Cancelar and gives it back to Eliminar when cancelled", () => {
    render(<DeleteAction name="Laptop" question="¿Seguro?" pending={false} onConfirm={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Eliminar Laptop" }));
    expect(screen.getByRole("button", { name: "Cancelar" })).toHaveFocus();
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(screen.getByRole("button", { name: "Eliminar Laptop" })).toHaveFocus();
  });

  it("disables both answers while the deletion is pending and calls onStart first", () => {
    const onStart = vi.fn();
    const { rerender } = render(<DeleteAction name="X" question="¿?" pending={false} onConfirm={() => {}} onStart={onStart} />);
    fireEvent.click(screen.getByRole("button", { name: "Eliminar X" }));
    expect(onStart).toHaveBeenCalledTimes(1);
    rerender(<DeleteAction name="X" question="¿?" pending onConfirm={() => {}} onStart={onStart} />);
    expect(screen.getByRole("button", { name: "Sí, eliminar" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
  });
});
