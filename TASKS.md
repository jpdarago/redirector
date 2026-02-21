# TASKS

## Instructions for Claude

This file contains tasks to do. I will write these as I come up with ideas at work or at home, these are intended for you to do.

1. For each new task, create a Github issue. Add the issue to the task in an initial commit.
1. Create a pull request with the commits you make to solve a task. Link the issue on that PR.
1. Investigation results should go into the issue.
1. If you need a clarification from me, write it in the issue, and I will reply there.
1. For each task, once you finish it, check the appropriate mark in this document and close the issue.

## Tasks

- [x] Add comment support. (Issue #5) The comments start with # and should be ignored for redirection, but they should be displayed in the listing page under each link. The contents should be rendered as Markdown without escaping (no need to escape them since we control that information). Example:

```md 
# Lyle McDonald - _The Protein Book_
https://jpdarago.com/books/link-to-the-book.pdf
```

- [x] Push the redirect links from a folder in Github. (Issue #7) I can grant an SSH key access to the `/usr/srv/redirects` folder so Github Actions uses that key and pushed it to the directory. The command should be `rsync`. The SSH key should only have permissions to write to that folder. The folder structure of the repo should be preserved. Update the Readme with setup instructions for this for me to do to set up the permissions on my server.
