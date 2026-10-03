# gogpu fork

This is a copy of [gogpu](https://github.com/gogpu/gogpu) **v0.54.0** (MIT,
see LICENSE), the windowing and GPU-surface layer nectar_ui runs on. It
lives here so nectar_ui can use platform features gogpu doesn't have yet;
every change is meant to be sent upstream, after which this copy can go
away again.

The copy has gogpu's non-test Go sources with the import path changed from
`github.com/gogpu/gogpu` to `github.com/minelifes/nectar_ui/internal/gogpu`
(commit "Vendor gogpu v0.54.0 as internal/gogpu"). Examples, docs, tests and
golden images are left out.

## Changes on top of v0.54.0

See the git history of this folder; each change is its own commit and is
listed here:

(none yet)

## Updating

1. Copy the new gogpu release over this folder (same file selection), and
   rewrite the import path again.
2. Re-apply the changes listed above that upstream hasn't merged.
