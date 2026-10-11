import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { useDeepLink } from "./useDeepLink";

afterEach(cleanup);

interface Item {
  id: number;
  name: string;
}
const ITEMS: Item[] = [
  { id: 1, name: "Uno" },
  { id: 2, name: "Dos" },
];

function Harness({ items, canOpen }: { items: Item[] | null; canOpen?: (i: Item) => boolean }) {
  const [open, setOpen] = useState<Item | null>(null);
  const { missing } = useDeepLink({ param: "editar", items: items ?? undefined, open, onOpen: setOpen, canOpen });
  const location = useLocation();
  return (
    <div>
      <p data-testid="url">{location.search}</p>
      <p data-testid="open">{open?.name ?? "none"}</p>
      <p data-testid="missing">{String(missing)}</p>
      <button type="button" onClick={() => setOpen(null)}>
        cerrar
      </button>
      <button type="button" onClick={() => setOpen(ITEMS[1])}>
        otro
      </button>
    </div>
  );
}

const renderAt = (search: string, items: Item[] | null = ITEMS, canOpen?: (i: Item) => boolean) =>
  render(
    <MemoryRouter initialEntries={[`/x${search}`]}>
      <Harness items={items} canOpen={canOpen} />
    </MemoryRouter>,
  );

describe("useDeepLink", () => {
  it("opens the record of the link and removes the parameter, keeping the others, when it closes", () => {
    renderAt("?mes=2026-10&editar=2");
    expect(screen.getByTestId("open")).toHaveTextContent("Dos");
    expect(screen.getByTestId("url")).toHaveTextContent("?mes=2026-10&editar=2");
    fireEvent.click(screen.getByRole("button", { name: "cerrar" }));
    expect(screen.getByTestId("url")).toHaveTextContent(/^\?mes=2026-10$/);
  });

  it("waits for the list to load", () => {
    renderAt("?editar=1", null);
    expect(screen.getByTestId("open")).toHaveTextContent("none");
    expect(screen.getByTestId("url")).toHaveTextContent("?editar=1");
  });

  it("an id that is not in the list leaves the page as it is, reports it and drops the parameter", () => {
    renderAt("?mes=2026-10&editar=99");
    expect(screen.getByTestId("open")).toHaveTextContent("none");
    expect(screen.getByTestId("missing")).toHaveTextContent("true");
    expect(screen.getByTestId("url")).toHaveTextContent(/^\?mes=2026-10$/);
  });

  it("ignores a malformed id and a record that cannot be opened", () => {
    renderAt("?editar=abc");
    expect(screen.getByTestId("missing")).toHaveTextContent("false");
    cleanup();
    renderAt("?editar=1", ITEMS, () => false);
    expect(screen.getByTestId("open")).toHaveTextContent("none");
    expect(screen.getByTestId("missing")).toHaveTextContent("true");
  });

  it("opening another record by hand also drops the parameter", () => {
    renderAt("?editar=1");
    expect(screen.getByTestId("open")).toHaveTextContent("Uno");
    fireEvent.click(screen.getByRole("button", { name: "otro" }));
    expect(screen.getByTestId("url")).toHaveTextContent("");
    expect(screen.getByTestId("open")).toHaveTextContent("Dos");
  });
});
