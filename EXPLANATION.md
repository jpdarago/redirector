I want this project to be a URL redirector that I will use on my personal server.

The URL redirector setup will work based on directories and files.

Let's say I ask it to look at this folder structure

```sh
❯ tree /tmp/example
/tmp/example
├── a.txt
├── b
│   ├── c.txt
│   └── d.txt
└── e
    └── f
        └── g.txt

3 directories, 4 files
```

```sh
❯ find /tmp/example -type f -print | while read -r f; do echo "=== Contents of $f ==="; cat $f; echo "==="; done
=== Contents of /tmp/example/a.txt ===
google.com
===
=== Contents of /tmp/example/e/f/g.txt ===
youtube.com
===
=== Contents of /tmp/example/b/c.txt ===
amazon.com
===
=== Contents of /tmp/example/b/d.txt ===
facebook.com
===
```

If I ask the redirector to look at that folder on port 8080, then

`localhost:8080/a` should go to `google.com`
`localhost:8080/b/c` should go to `amazon.com`
`localhost:8080/b/d` should go to `facebook.com`
`localhost:8080/e/f/g` should go to `youtube.com`

Using 301 redirect. Everything else should return 404 not found.

It should only look at txt files.

If I add new files manually it should still work without rebooting the server.
