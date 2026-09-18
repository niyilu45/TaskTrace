# teamData

TaskTrace creates the local team collaboration repository in this folder at runtime.
Shared task snapshots, attachments, member notifications, task links, and repository metadata are
runtime data and are intentionally excluded from Git.

To collaborate over a LAN, share the packaged program's `teamData` folder with the
required Windows users. TaskTrace reads the folder's Windows permissions to suggest
member names and places the detected UNC path in generated task links.
