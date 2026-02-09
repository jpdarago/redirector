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
└── e/
    └── f/
        └── g.txt  # contains "youtube.com"
```

The server produces these redirects:

| Request | Redirect (301) |
|---|---|
| `GET /a` | `https://google.com` |
| `GET /b/c` | `https://amazon.com` |
| `GET /b/d` | `https://facebook.com` |
| `GET /e/f/g` | `https://youtube.com` |
| anything else | 404 |

URLs without a scheme get `https://` prepended automatically. URLs that already contain `://` are used as-is.

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
GOOS=linux GOARCH=amd64 go build -o redirector .
scp redirector your-server:/tmp/
ssh your-server 'sudo mv /tmp/redirector /usr/local/bin/ && sudo systemctl restart redirector'
```

Create the redirects directory:

```sh
sudo mkdir -p /srv/redirects
```

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

## Development

Requires [devenv](https://devenv.sh/):

```sh
devenv shell    # enter dev environment with Go
devenv up       # run the server
```
