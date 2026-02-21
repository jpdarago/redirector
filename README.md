# redirector

A file-based URL redirector. Define redirects as `.txt` files in a directory — the file path becomes the route and its content becomes the redirect target.

## How it works

Given a directory like:

```
/srv/redirects/
├── a.txt          # contains "google.com"
├── b/
│   ├── c.txt      # contains "amazon.com"
│   └── d.txt      # contains "facebook.com"
├── e/
│   └── f/
│       └── g.txt  # contains "youtube.com"
├── books/
│   └── protein.txt
└── todo/
    └── _index.txt # contains "todoist.com"
```

Files contain the redirect target URL, optionally preceded by comment lines starting with `#`:

```
# Lyle McDonald - _The Protein Book_
https://jpdarago.com/books/link-to-the-book.pdf
```

Comments are ignored for redirection but displayed as descriptions on the listing page, rendered as Markdown.

The server produces these redirects:

| Request | Redirect (301) |
|---|---|
| `GET /a` | `https://google.com` |
| `GET /b/c` | `https://amazon.com` |
| `GET /b/d` | `https://facebook.com` |
| `GET /e/f/g` | `https://youtube.com` |
| `GET /todo` | `https://todoist.com` |
| `GET /todo/` | `https://todoist.com` |
| `GET /books/protein` | `https://jpdarago.com/books/link-to-the-book.pdf` |
| anything else | 404 |

URLs without a scheme get `https://` prepended automatically. URLs that already contain `://` are used as-is.

Trailing slashes are accepted — `GET /a/` resolves the same as `GET /a`.

A file named `_index.txt` acts as a directory index: its parent directory becomes the route key. This lets you use `todo/_index.txt` to handle requests to `/todo`. A root-level `_index.txt` is ignored since `/` serves the route listing page.

Routes reload from disk every 100ms — add or remove files without restarting the server.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `REDIRECT_DIR` | yes | — | Path to the directory containing `.txt` redirect files |
| `PORT` | no | `8080` | Port to listen on |

## Usage

```sh
REDIRECT_DIR=/srv/redirects go run .
```

## Deployment

Build the binary and copy it to the server:

```sh
go build -o redirector .
sudo cp redirector /usr/local/bin/
```

### Cross-compilation

If you're building on a different OS or architecture (e.g. a NixOS laptop deploying to a Debian amd64 server), cross-compile and copy the binary over:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o redirector .
scp redirector your-server:/tmp/
ssh your-server 'sudo mv /tmp/redirector /usr/local/bin/ && sudo systemctl restart redirector'
```

Create the redirects directory:

```sh
sudo mkdir -p /srv/redirects
sudo chown "$USER:$USER" /srv/redirects
sudo chmod o+rX /srv/redirects
```

This makes your user the owner (so you can add and edit redirect files) while keeping the directory readable by the service, which runs as a sandboxed dynamic user.

Install the systemd service:

```sh
sudo cp redirector.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now redirector
```

Check status and logs:

```sh
sudo systemctl status redirector
sudo journalctl -u redirector -f
```

Edit `REDIRECT_DIR` and `PORT` in the service file to match your setup. The service runs with hardened settings (read-only filesystem, no root privileges, private /tmp).

### nginx reverse proxy

To serve the redirector under the `/go/` route of an existing nginx site, add the following to your server block:

```nginx
location /go/ {
    proxy_pass http://127.0.0.1:8080/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

The trailing slash on both `location /go/` and `proxy_pass http://127.0.0.1:8080/` is important — it strips the `/go/` prefix so that `/go/a` is forwarded to the redirector as `/a`.

Test and reload nginx:

```sh
sudo nginx -t && sudo systemctl reload nginx
```

### Syncing redirects from GitHub

Redirect `.txt` files live in the `redirects/` directory of this repository. A GitHub Actions workflow automatically rsyncs them to the server on every push to `main` that changes files under `redirects/`.

**Importing existing redirects from the server:**

```sh
rsync -avz jpdarago@jpdarago.com:/srv/redirects/ redirects/
```

This copies all `.txt` files from the server into the `redirects/` directory. Review the result, commit, and push to make the repo the source of truth going forward.

**Server setup:**

1. Generate a dedicated Ed25519 SSH key pair. Either:

   - **Bitwarden**: Use the SSH key generator in Bitwarden to create an Ed25519 key and export the public/private keys.
   - **ssh-keygen**:
     ```sh
     ssh-keygen -t ed25519 -f deploy_redirects -N ""
     ```

2. Ensure `rrsync` is installed on the server. Check with:

   ```sh
   which rrsync
   ```

   On Debian/Ubuntu it ships with the `rsync` package at `/usr/bin/rrsync`. If missing, install `rsync` (`sudo apt install rsync`). On other distros you may need to install it separately.

3. Add the **public** key to `~jpdarago/.ssh/authorized_keys` with restrictions so it can only rsync into `/srv/redirects`:

   ```
   command="/usr/bin/rrsync /srv/redirects",restrict ssh-ed25519 AAAA... github-actions-deploy
   ```

   This prevents shell access, port forwarding, and any command other than rsync into the specified directory.

4. Add the **private** key as a GitHub repository secret named `DEPLOY_SSH_KEY`.

5. Ensure `/srv/redirects` is owned by `jpdarago` and writable:

   ```sh
   sudo chown jpdarago:jpdarago /srv/redirects
   sudo chmod 750 /srv/redirects
   ```

Optionally set `DEPLOY_HOST` and `DEPLOY_USER` secrets to override the defaults (`jpdarago.com` and `jpdarago`).

**Verifying the setup locally before using the workflow:**

Save the private key to a temporary file and run through these checks:

1. Verify SSH connectivity with the restricted key:

   ```sh
   ssh -i /path/to/deploy_key jpdarago@jpdarago.com echo "SSH works"
   ```

   This should fail or print an `rrsync` error (not a shell prompt) — that confirms the key is restricted.

2. Test a dry-run rsync to verify permissions:

   ```sh
   rsync -avz --dry-run -e "ssh -i /path/to/deploy_key" redirects/ jpdarago@jpdarago.com:/srv/redirects/
   ```

   You should see the list of files that would be transferred with no permission errors.

3. Do an actual rsync to confirm it works end to end:

   ```sh
   rsync -avz -e "ssh -i /path/to/deploy_key" redirects/ jpdarago@jpdarago.com:/srv/redirects/
   ```

4. Verify the restricted key cannot run arbitrary commands:

   ```sh
   ssh -i /path/to/deploy_key jpdarago@jpdarago.com ls /
   ```

   This should be rejected by `rrsync`.

## Development

Requires [devenv](https://devenv.sh/):

```sh
devenv shell    # enter dev environment with Go
devenv up       # run the server
```
