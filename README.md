# Deutsch Karten

Anki-style flash cards for learning German. Written in Go using only the standard library.

## Run

```sh
go run .                       # http://localhost:8082, data in ./cards.json
go run . -addr :9000 -db ~/german.json
go build -o anki . && ./anki   # single binary; templates and assets are embedded
```

### Login

Set `ANKI_PASSWORD` to require a password (sessions last 30 days):

```sh
ANKI_PASSWORD=geheim go run .
ANKI_PASSWORD=geheim ./anki -secure-cookies   # when served over HTTPS
```

Without it the app is open to anyone who can reach it.

### Server install

`sudo ./deploy/install.sh` installs a systemd service (port 8082, data in
`/var/lib/anki`). On first run it asks for the password and stores it in
`/etc/anki/anki.env`; to change it, edit that file and `sudo systemctl restart anki`.

## Features

- **Add cards**: German word or phrase, translation, example sentence, notes, deck. Buttons insert ä ö ü ß.
- **Import text**: paste one card per line, e.g. `der Hund ; the dog ; Der Hund bellt.`
  (separators: `;` `|` tab `=` ` - `; lines starting with `#` are skipped).
- **Study**: spaced repetition (simplified SM-2, like Anki) with Again / Hard / Good / Easy.
  Study DE → EN or EN → DE. Keyboard: `Space` shows the answer, `1`–`4` rate it.
- **Pronunciation**: 🔊 reads German aloud using the browser's text-to-speech.
- **Browse**: search, edit, reset progress, delete; export everything as JSON.
# anki
