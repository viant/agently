Applies a simplified, file‑oriented patch to files under workdir. Calls are stateless; all paths in the patch must be relative to workdir. Absolute paths are rejected.

Format

- Envelope:
  *** Begin Patch
  [one or more file sections]
  *** End Patch
- Operations:
    - *** Add File:  — creates a new file; each subsequent line must start with “+”.
    - *** Delete File:  — removes an existing file.
    - *** Update File:  — patches an existing file in place.
    - Optional rename after Update: *** Move to:
- Hunks:
    - Introduced by @@ [header]
    - Hunk lines start with:
    - " " context (unchanged)
    - "+" inserted text
    - "-" removed text
- For truncated hunks, you may end with "*** End of File".

Rules
- When operating of file system 
  - Never guess workdir - always confirm with user
  - Never use workdir as '.'
  - Paths are relative to workdir; absolute paths are rejected with a corrective error.
  - Parent directories for Add/Move targets are created as needed inside workdir.
  - Update/Delete fail if the target does not exist.
- The tool validates structure and stops on the first structural error, returning a helpful message.


Output
- Returns status and counts of lines added/removed for the applied patch; includes an error message on failure.

Example
*** Begin Patch
*** Add File: hello.txt
+Hello, world!
*** Update File: src/app.py
@@
-print("Hi")
+print("Hello, world!")
*** Move to: src/main.py
*** End Patch

Proposed description (JSON-safe):
"Applies a simplified, file-oriented patch to files under workdir. Calls are stateless; all paths in the patch must be relative to workdir. Absolute paths are rejected.\n\nFormat\n- Envelope:\n  *** Begin Patch\n
[one or more file sections]\n  *** End Patch\n- Operations:\n  - *** Add File:  — creates a new file; each subsequent line must start with "+".\n  - *** Delete File:  — removes an existing file.\n  - *** Update
File:  — patches an existing file in place.\n  - Optional rename after Update: *** Move to: \n- Hunks:\n  - Introduced by @@ [header]\n  - Hunk lines start with:\n    - " " context (unchanged)\n    - "+" inserted
text\n    - "-" removed text\n  - For truncated hunks, you may end with "*** End of File".\n\nRules\n- Paths are relative to workdir; absolute paths are rejected with a corrective error.\n- Parent directories
for Add/Move targets are created as needed inside workdir.\n- Update/Delete fail if the target does not exist.\n- The tool validates structure and stops on the first structural error, returning a helpful message.
\n\nOutput\n- Returns status and counts of lines added/removed for the applied patch; includes an error message on failure.\n\nExample\n*** Begin Patch\n*** Add File: hello.txt\n+Hello, world!\n*** Update File:
src/app.py\n@@\n-print("Hi")\n+print("Hello, world!")\n*** Move to: src/main.py\n*** End Patch"
