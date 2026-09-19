/**
 * The words on the delete confirmation. A delete is final and silent: the
 * event leaves the calendar and every feed, a subscribed calendar drops it on
 * its next refresh, and nobody is told it was called off. Cancelling is the
 * opposite on both counts, so the dialog points there for a real event.
 */
export function deleteConfirmation(title: string, published: boolean, recurring: boolean) {
  if (!published) {
    return {
      title: 'Delete this draft?',
      message: `"${title}" has never been published, so nothing has it yet. Deleting it cannot be undone.`,
      confirmLabel: 'Delete draft',
    };
  }
  const what = recurring ? `"${title}" and every one of its occurrences are` : `"${title}" is`;
  return {
    title: recurring ? 'Delete this series for good?' : 'Delete this event for good?',
    message: `${what} removed from the calendar and from every feed. Subscribed calendars drop it the next time they refresh, and nobody is told it was called off. To tell people an event is off, cancel it instead. Deleting cannot be undone.`,
    confirmLabel: recurring ? 'Delete the series' : 'Delete the event',
  };
}
