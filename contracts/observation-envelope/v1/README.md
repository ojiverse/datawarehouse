# Observation Envelope v1 Contract

`schema.json` が Observation Envelope v1 の authoritative physical JSON contract である。

Go / TypeScript の struct / type はこの schema の implementation artifact であり、独立した schema authority ではない。

## Files

* `schema.json`: JSON Schema Draft 2020-12
* `http-backfill-page.json`: HTTP Backfill page の representative fixture

fixture は schema に適合し、Go / TypeScript 双方の compatibility test で使用する。

JSON field order や whitespace の byte equality は contract に含めず、semantic JSON equivalence を検証する。
