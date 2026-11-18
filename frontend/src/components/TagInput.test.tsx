import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createRef, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { TagInput, type TagInputHandle } from "./TagInput";

afterEach(cleanup);

function Harness({ initial = [], handleRef, invalidKeyword }: { initial?: string[]; handleRef?: React.Ref<TagInputHandle>; invalidKeyword?: string }) {
  const [value, setValue] = useState<string[]>(initial);
  return (
    <>
      <TagInput label="Palabras clave" value={value} onChange={setValue} handleRef={handleRef} invalidKeyword={invalidKeyword} invalidMessage="La palabra clave «heb» ya pertenece a la categoría Súper." />
      <pre data-testid="out">{JSON.stringify(value)}</pre>
    </>
  );
}

const field = () => screen.getByLabelText("Palabras clave");
const out = () => JSON.parse(screen.getByTestId("out").textContent ?? "[]") as string[];

describe("TagInput", () => {
  it("renders a removable chip per keyword with an accessible label", () => {
    render(<Harness initial={["heb", "café"]} />);
    expect(screen.getByRole("button", { name: "Quitar heb" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Quitar café" }));
    expect(out()).toEqual(["heb"]);
  });

  it("adds a chip with Enter and with a comma, trimming the text", () => {
    render(<Harness />);
    fireEvent.change(field(), { target: { value: "  heb " } });
    fireEvent.keyDown(field(), { key: "Enter" });
    fireEvent.change(field(), { target: { value: "walmart" } });
    fireEvent.keyDown(field(), { key: "," });
    expect(out()).toEqual(["heb", "walmart"]);
    expect(field()).toHaveValue("");
  });

  it("splits a pasted list into chips", () => {
    render(<Harness />);
    fireEvent.paste(field(), { clipboardData: { getData: () => "a, b, c" } });
    expect(out()).toEqual(["a", "b", "c"]);
  });

  it("removes the last chip with Backspace on an empty field only", () => {
    render(<Harness initial={["a", "b"]} />);
    fireEvent.change(field(), { target: { value: "x" } });
    fireEvent.keyDown(field(), { key: "Backspace" });
    expect(out()).toEqual(["a", "b"]);
    fireEvent.change(field(), { target: { value: "" } });
    fireEvent.keyDown(field(), { key: "Backspace" });
    expect(out()).toEqual(["a"]);
  });

  it("ignores duplicates ignoring case and accents and says so", () => {
    render(<Harness initial={["café"]} />);
    fireEvent.change(field(), { target: { value: "CAFE" } });
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(out()).toEqual(["café"]);
    expect(screen.getByRole("status")).toHaveTextContent("Ya estaba: CAFE.");
    fireEvent.change(field(), { target: { value: "t" } });
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("commits the pending text on blur", () => {
    render(<Harness />);
    fireEvent.change(field(), { target: { value: "pendiente" } });
    fireEvent.blur(field());
    expect(out()).toEqual(["pendiente"]);
  });

  it("commit() through the handle returns the list with the pending text folded in", () => {
    const ref = createRef<TagInputHandle>();
    render(<Harness initial={["a"]} handleRef={ref} />);
    fireEvent.change(field(), { target: { value: "b" } });
    expect(ref.current?.commit()).toEqual(["a", "b"]);
    expect(ref.current?.commit()).toEqual(["a", "b"]);
  });

  it("marks the rejected chip and shows the message next to it", () => {
    render(<Harness initial={["heb", "otro"]} invalidKeyword="HEB" />);
    expect(screen.getByText("heb").closest("li")).toHaveAttribute("data-invalid", "true");
    expect(screen.getByText("otro").closest("li")).not.toHaveAttribute("data-invalid");
    expect(screen.getByRole("alert")).toHaveTextContent("ya pertenece a la categoría Súper");
  });
});
