// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PromptDialog } from './ConfirmDialog';

afterEach(() => cleanup());

describe('PromptDialog', () => {
  it('stays open with the typed name and says why when the save is refused', async () => {
    const onCancel = vi.fn();
    render(
      <PromptDialog
        title="Rename calendar"
        message="Pick a name."
        label="Calendar name"
        initialValue="Exo"
        confirmLabel="Rename"
        minLength={2}
        maxLength={80}
        onCancel={onCancel}
        onConfirm={async () => {
          throw new Error('A calendar with that name already exists.');
        }}
      />,
    );
    const input = screen.getByLabelText('Calendar name');
    fireEvent.change(input, { target: { value: 'EMG Fabric' } });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Rename' }));
    });
    expect(screen.getByRole('alert').textContent).toBe('A calendar with that name already exists.');
    expect((screen.getByLabelText('Calendar name') as HTMLInputElement).value).toBe('EMG Fabric');
    expect(onCancel).not.toHaveBeenCalled();
  });
});
