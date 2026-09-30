import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EDIT_HIGHLIGHT_CLASS, EDIT_HIGHLIGHT_INSET_CLASS, EDIT_HIGHLIGHT_MS, useFocusOnEdit } from "./useFocusOnEdit";

function Probe({ active, scroll, inset }: { active: boolean; scroll?: boolean; inset?: boolean }) {
  const ref = useFocusOnEdit<HTMLFormElement>(active, { scroll, inset });
  return (
    <form ref={ref} data-testid="form">
      <input aria-label="hidden" type="hidden" />
      <input aria-label="first" disabled />
      <input aria-label="second" />
    </form>
  );
}

function mockReducedMotion(matches: boolean) {
  window.matchMedia = vi.fn().mockReturnValue({ matches }) as unknown as typeof window.matchMedia;
}

describe("useFocusOnEdit", () => {
  const scrollIntoView = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers();
    scrollIntoView.mockClear();
    Element.prototype.scrollIntoView = scrollIntoView;
    mockReducedMotion(false);
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("does nothing while inactive", () => {
    const { getByTestId } = render(<Probe active={false} />);
    expect(scrollIntoView).not.toHaveBeenCalled();
    expect(getByTestId("form")).not.toHaveClass(EDIT_HIGHLIGHT_CLASS);
  });

  it("scrolls smoothly, focuses the first enabled field and highlights, then clears after the timeout", () => {
    const { getByTestId, getByLabelText } = render(<Probe active />);
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
    expect(getByLabelText("second")).toHaveFocus();
    expect(getByTestId("form")).toHaveClass(EDIT_HIGHLIGHT_CLASS);
    act(() => {
      vi.advanceTimersByTime(EDIT_HIGHLIGHT_MS - 1);
    });
    expect(getByTestId("form")).toHaveClass(EDIT_HIGHLIGHT_CLASS);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(getByTestId("form")).not.toHaveClass(EDIT_HIGHLIGHT_CLASS);
  });

  it("scrolls instantly when the user prefers reduced motion", () => {
    mockReducedMotion(true);
    render(<Probe active />);
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "auto", block: "start" });
  });

  it("skips scrolling when asked and supports the inset variant", () => {
    const { getByTestId } = render(<Probe active scroll={false} inset />);
    expect(scrollIntoView).not.toHaveBeenCalled();
    expect(getByTestId("form")).toHaveClass(EDIT_HIGHLIGHT_CLASS, EDIT_HIGHLIGHT_INSET_CLASS);
  });

  it("re-triggers when active flips back on", () => {
    const { rerender, getByTestId } = render(<Probe active />);
    rerender(<Probe active={false} />);
    expect(getByTestId("form")).not.toHaveClass(EDIT_HIGHLIGHT_CLASS);
    rerender(<Probe active />);
    expect(getByTestId("form")).toHaveClass(EDIT_HIGHLIGHT_CLASS);
    expect(scrollIntoView).toHaveBeenCalledTimes(2);
  });
});
