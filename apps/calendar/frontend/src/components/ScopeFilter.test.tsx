// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Scope } from '../types';
import { ScopeFilter } from './ScopeFilter';

afterEach(() => cleanup());

describe('ScopeFilter', () => {
  const scope = (id: string, kind: Scope['kind'], name: string, parent_id?: string): Scope => ({
    id,
    kind,
    name,
    slug: id,
    path: name,
    slug_path: id,
    status: 'ACTIVE',
    parent_id,
    child_count: 0,
    event_count: 0,
  });

  it('puts every row of a list in a list item, the General row too', () => {
    const { container } = render(
      <ScopeFilter
        scopes={[scope('t', 'TEAM', 'BioTron'), scope('p', 'PROJECT', 'Exo', 't')]}
        selected={[]}
        onChange={vi.fn()}
      />,
    );
    const lists = container.querySelectorAll('ul');
    expect(lists.length).toBeGreaterThan(1);
    for (const list of lists) {
      for (const child of list.children) expect(child.tagName).toBe('LI');
    }
    expect(screen.getAllByText('General').length).toBeGreaterThan(0);
  });
});
