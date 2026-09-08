# onsetter

A Claude Code hook that injects prose from a `CLAUDE.md` at the Write or Edit
that trips its gate. See `README.md` for what an ask is, or run
`onsetter headers` for the header reference.

`make wire` installs the binary and writes the settings entry; plain `make`
only builds and tests. `go test ./...` is what CI runs.

## Asks

The two blocks below govern this repo. Both were measured with
`onsetter replay 'cmd/**/*.go' 'internal/**/*.go'` before being wired, and
narrowed until they stopped reaching files they were not about. Re-run it after
changing either one.

```ask
in: cmd/**/*.go
not-in: **/main.go
not-in: **/*_test.go
when: os\.Exit\(|log\.Fatal|panic\(

Everything reachable from `onsetter hook` stands in front of every Write and
Edit in a session, and the only acceptable failure there is exit 0 with no
output — bad JSON, a missing path, an unparseable block, a panic. Is this path
reachable from the hook, and if it is, does something above it recover and exit
0? If this is a subcommand that only ever runs from a terminal, continue.
```

`main.go` is excluded because exiting non-zero is what a CLI entry point is
for; the invariant is about the hook path only. A third candidate gating on
`additionalContext` also measured at 10%, but it fires on the same file as this
one, so wiring both would have meant two questions on every edit to `hook.go`.
The one-emitter rule is pinned by a test instead.

```ask
in: ask/ask.go
when: case "[a-z-]+":

Adding a header means four places have to agree: this switch, the `Headers`
slice that fixes the order `Match` applies gates and `replay` reports a funnel
in, a section plus a table row in `headers.md`, and the header list in
`skill.md`'s frontmatter, which is the only text deciding whether the skill
surfaces at all. Tests assert the last three, so a miss here fails CI rather
than shipping — but the parse error a reader gets is much better if all four
land together. If this case is not a new header, continue.
```
