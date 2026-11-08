// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import SceneBoundary from './SceneBoundary';

afterEach(() => cleanup());

function BrokenScene(): never {
  throw new Error('Could not load /hdri/empty_warehouse_01_1k.hdr');
}

describe('SceneBoundary', () => {
  it('shows the scene while it works', () => {
    render(
      <SceneBoundary fallback={<p>Stacked</p>}>
        <p>Scene</p>
      </SceneBoundary>,
    );
    expect(screen.getByText('Scene')).toBeTruthy();
  });

  it('shows the fallback instead of a blank page when the scene throws', () => {
    // React reports the caught error to the console; keep the test output quiet.
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    render(
      <SceneBoundary fallback={<p>Stacked</p>}>
        <BrokenScene />
      </SceneBoundary>,
    );
    expect(screen.getByText('Stacked')).toBeTruthy();
    expect(warn).toHaveBeenCalled();
    vi.restoreAllMocks();
  });
});
