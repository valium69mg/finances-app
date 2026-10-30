import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Collapsible } from "./Collapsible";

afterEach(cleanup);

describe("Collapsible", () => {
  it("is collapsed by default and does not render its content", () => {
    render(
      <Collapsible title="Detalle">
        <p>contenido</p>
      </Collapsible>,
    );
    const button = screen.getByRole("button", { name: "Detalle" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("contenido")).not.toBeInTheDocument();
  });

  it("starts open with defaultOpen and wires aria-controls to its region", () => {
    render(
      <Collapsible title="Detalle" defaultOpen>
        <p>contenido</p>
      </Collapsible>,
    );
    const button = screen.getByRole("button", { name: "Detalle" });
    expect(button).toHaveAttribute("aria-expanded", "true");
    const region = screen.getByRole("region", { name: "Detalle" });
    expect(button.getAttribute("aria-controls")).toBe(region.id);
    expect(screen.getByText("contenido")).toBeInTheDocument();
  });

  it("toggles on click and rotates the chevron", () => {
    render(
      <Collapsible title="Detalle">
        <p>contenido</p>
      </Collapsible>,
    );
    const button = screen.getByRole("button", { name: "Detalle" });
    const chevron = button.querySelector("svg");
    expect(chevron?.getAttribute("class")).not.toContain("rotate-180");
    fireEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("contenido")).toBeInTheDocument();
    expect(chevron?.getAttribute("class")).toContain("rotate-180");
    fireEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("contenido")).not.toBeInTheDocument();
  });

  it("only animates when motion is allowed", () => {
    render(
      <Collapsible title="Detalle">
        <p>contenido</p>
      </Collapsible>,
    );
    expect(screen.getByRole("button").querySelector("svg")?.getAttribute("class")).toContain("motion-reduce:transition-none");
  });

  it("is a native button, so keyboard activation is the browser's own", () => {
    render(
      <Collapsible title="Detalle">
        <p>contenido</p>
      </Collapsible>,
    );
    const button = screen.getByRole("button", { name: "Detalle" });
    expect(button.tagName).toBe("BUTTON");
    expect(button).toHaveAttribute("type", "button");
  });

  it("can be controlled and reports changes", () => {
    const onOpenChange = vi.fn();
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <Collapsible
          title="Detalle"
          open={open}
          onOpenChange={(v) => {
            onOpenChange(v);
            setOpen(v);
          }}
        >
          <p>contenido</p>
        </Collapsible>
      );
    }
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Detalle" }));
    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.getByText("contenido")).toBeInTheDocument();
  });

  it("shows the optional summary and uses the requested heading level", () => {
    render(
      <Collapsible title="Detalle" summary="3 filas" as="h2">
        <p>contenido</p>
      </Collapsible>,
    );
    expect(screen.getByRole("heading", { level: 2 })).toBeInTheDocument();
    expect(screen.getByText("3 filas")).toBeInTheDocument();
  });
});
