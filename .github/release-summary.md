Grasshopper 2.11.2

Memory view fixes.

- Archived memories stay listed after their project's last live memory is
  archived, and a new project's suggestions show right away.
- An edit kept across signing in again opens as a conflict if an agent changed
  the memory meanwhile, instead of saving over it.
- Reloading or closing the tab asks before losing an unsaved edit, and the
  editor no longer resets every 15 seconds while offline.
- Back and Forward over an opened memory keep the list loaded.
- Keyboard focus stays in the list after Keep, Discard, Restore and Show more.
- Keep all asks first; Keep and replace offers Undo; Undo stops retrying after
  a conflict and is announced once.
- Searching again from a project search stays in that project.
- Approve, Deny and Disconnect send once on a double click.
- The setup page opens a bare localhost address over HTTP.
