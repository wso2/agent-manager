# Embedded API reference viewer

`redoc-<version>.standalone.js.gz` is `bundles/redoc.standalone.js` from the npm package [`redoc`](https://www.npmjs.com/package/redoc), gzipped (`gzip -9 -n`). Redoc is MIT licensed (`LICENSE.redoc`); the libraries bundled in the script are listed with their licences in `redoc-<version>.standalone.js.LICENSE.txt` (MIT, and DOMPurify under Apache-2.0 / MPL-2.0).

| Version | SHA-256 of the uncompressed script |
|---|---|
| 2.5.4 | `dcaf76612bc4a3fbcc923a8966dee2f6146a5f32e5ce1b6f02dd60cbbf89500b` |

To update it:

1. `npm pack redoc@<version>` and unpack it in an empty directory.
2. `gzip -9 -n -c package/bundles/redoc.standalone.js > redoc-<version>.standalone.js.gz`, and copy `package/LICENSE` to `LICENSE.redoc` and `package/bundles/redoc.standalone.js.LICENSE.txt` to `redoc-<version>.standalone.js.LICENSE.txt`. Remove the old version's files.
3. Set `RedocVersion` in `../apidocs.go` and `bundleSHA256` in `../apidocs_test.go`, update the table above, and check the licences of any newly bundled library.
4. Open `/api/docs` with the browser's network panel open: every request must go to the server itself.
