import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DescriptionField } from "./DescriptionField";

afterEach(cleanup);

const owners = [
  { keyword: "heb", category: "Súper" },
  { keyword: "hbo", category: "Suscripciones" },
  { keyword: "uber", category: "Transporte" },
];

function Harness({ initial = "", onPick = vi.fn() }: { initial?: string; onPick?: (o: { keyword: string; category: string }) => void }) {
  const [value, setValue] = useState(initial);
  return <DescriptionField value={value} onChange={setValue} owners={owners} onPick={onPick} />;
}

const input = () => screen.getByLabelText("Descripción");

describe("DescriptionField", () => {
  it("suggests nothing for an empty description", () => {
    render(<Harness />);
    expect(screen.queryByRole("group", { name: "Palabras clave sugeridas" })).not.toBeInTheDocument();
  });

  it("shows the matching keywords with their category", () => {
    render(<Harness />);
    fireEvent.change(input(), { target: { value: "H" } });
    expect(screen.getByRole("button", { name: /heb\s*·\s*Súper/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /hbo\s*·\s*Suscripciones/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /uber/ })).not.toBeInTheDocument();
  });

  it("picking completes a prefix, reports the owner and closes the list", () => {
    const onPick = vi.fn();
    render(<Harness onPick={onPick} />);
    fireEvent.change(input(), { target: { value: "he" } });
    fireEvent.click(screen.getByRole("button", { name: /heb/ }));
    expect(input()).toHaveValue("heb");
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ keyword: "heb", category: "Súper" }));
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
  });

  it("leaves a longer description untouched when picking", () => {
    const onPick = vi.fn();
    render(<Harness initial="compra en heb semanal" onPick={onPick} />);
    fireEvent.click(screen.getByRole("button", { name: /heb/ }));
    expect(input()).toHaveValue("compra en heb semanal");
    expect(onPick).toHaveBeenCalledOnce();
  });

  it("ArrowDown enters the chips, arrows move, Escape closes and focuses the input", () => {
    render(<Harness />);
    fireEvent.change(input(), { target: { value: "h" } });
    input().focus();
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    const [first, second] = screen.getAllByRole("button");
    expect(first).toHaveFocus();
    fireEvent.keyDown(first, { key: "ArrowRight" });
    expect(second).toHaveFocus();
    fireEvent.keyDown(second, { key: "ArrowLeft" });
    expect(first).toHaveFocus();
    fireEvent.keyDown(first, { key: "Escape" });
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
    expect(input()).toHaveFocus();
    // Typing again brings the suggestions back.
    fireEvent.change(input(), { target: { value: "hb" } });
    expect(screen.getByRole("button", { name: /hbo/ })).toBeInTheDocument();
  });

  it("Escape in the input closes the list and stops there", () => {
    const onKey = vi.fn();
    render(
      <div onKeyDown={onKey}>
        <Harness />
      </div>,
    );
    fireEvent.change(input(), { target: { value: "h" } });
    fireEvent.keyDown(input(), { key: "Escape" });
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
    expect(onKey).not.toHaveBeenCalled();
  });
});
