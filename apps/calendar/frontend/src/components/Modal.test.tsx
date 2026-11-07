// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Modal } from './Modal';

afterEach(() => cleanup());

// The event details, then a confirmation opened over them, as App does it.
function Stacked({ onCloseDetails }: { onCloseDetails: () => void }) {
  const [confirming, setConfirming] = useState(false);
  return (
    <Modal title="General meeting" onClose={onCloseDetails}>
      <button type="button" onClick={() => setConfirming(true)}>
        Cancel this occurrence
      </button>
      {confirming && (
        <Modal title="Cancel this occurrence?" onClose={() => setConfirming(false)}>
          <p>Confirm</p>
        </Modal>
      )}
    </Modal>
  );
}

function openStack(onCloseDetails = vi.fn()) {
  render(<Stacked onCloseDetails={onCloseDetails} />);
  fireEvent.click(screen.getByRole('button', { name: 'Cancel this occurrence' }));
  return onCloseDetails;
}

describe('Modal', () => {
  it('names each stacked dialog by its own title', () => {
    openStack();
    const [details, confirm] = screen.getAllByRole('dialog');
    expect(details?.getAttribute('aria-labelledby')).not.toBe(
      confirm?.getAttribute('aria-labelledby'),
    );
    const name = (dialog?: HTMLElement) =>
      document.getElementById(dialog?.getAttribute('aria-labelledby') ?? '')?.textContent;
    expect(name(details)).toBe('General meeting');
    expect(name(confirm)).toBe('Cancel this occurrence?');
  });

  it('closes only the top dialog on Escape', () => {
    const onCloseDetails = openStack();
    act(() => {
      fireEvent.keyDown(window, { key: 'Escape' });
    });
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    expect(screen.queryByText('Confirm')).toBeNull();
    expect(onCloseDetails).not.toHaveBeenCalled();

    act(() => {
      fireEvent.keyDown(window, { key: 'Escape' });
    });
    expect(onCloseDetails).toHaveBeenCalledTimes(1);
  });
});
