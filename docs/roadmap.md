# Peekd Roadmap

These are practical follow-up features identified for the file server. They
are intentionally separate from the current preview implementation.

### Archive preview limits

Bound archive entry counts, metadata scanning, and compressed input processing.
Reject or mark unsafe absolute and `../` entry paths without extracting files.

### Directory search and filtering

Add client-side filtering for the current directory, with server-side search
as a later option for large trees.

### XML preview

Add a bounded, safe XML text or tree preview. Disable external entities and
network access, and never render embedded HTML.
