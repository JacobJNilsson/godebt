# godebt comparison

Before: index 0/100, scoring 0.3.0.
After: index 5/100, scoring 0.3.0.
The index delta is +5.

## Metrics

| metric | before | after | delta |
| --- | ---: | ---: | ---: |
| complexity.cc.max.production | 1 | 15 | +14 |
| complexity.cc.max.test | 2 | 12 | +10 |
| complexity.cc.p50.production | 1 | 1 | 0 |
| complexity.cc.p50.test | 2 | 2 | 0 |
| complexity.cc.p90.production | 1 | 11 | +10 |
| complexity.cc.p90.test | 2 | 12 | +10 |
| complexity.functions.production | 5 | 23 | +18 |
| complexity.functions.test | 1 | 3 | +2 |
| duplication.density.production | 0 | 0 | 0 |
| duplication.density.test | 0 | 0 | 0 |
| duplication.duplicated-lines.production | 0 | 0 | 0 |
| duplication.duplicated-lines.test | 0 | 0 | 0 |
| duplication.groups.production | 0 | 0 | 0 |
| duplication.groups.test | 0 | 0 | 0 |
| erosion.eroded-count.production | 0 | 3 | +3 |
| erosion.eroded-count.test | 0 | 1 | +1 |
| erosion.eroded-share.production | 0 | 0.5179 | +0.5179 |
| erosion.eroded-share.test | 0 | 0.7713 | +0.7713 |
| erosion.excess-mass.production | 0 | 17.73 | +17.73 |
| erosion.excess-mass.test | 0 | 4 | +4 |
| erosion.excess-share.production | 0 | 0.07982 | +0.07982 |
| erosion.excess-share.test | 0 | 0.1285 | +0.1285 |
| erosion.mass.production | 5 | 222.1 | +217.1 |
| erosion.mass.test | 4.472 | 31.12 | +26.65 |

## New findings

| set | kind | path | lines | identity |
| --- | --- | --- | ---: | --- |
| production | complexity.hotspot | cc.go | 35-70 | .:Long |
| production | complexity.hotspot | cc.go | 73-76 | .:Short |
| production | complexity.hotspot | cc.go | 79-81 | .:wrap#1 |
| test | complexity.hotspot | functions_test.go | 14-17 | .:allSet |

## Resolved findings

None.

## Changed findings

None.

Persistent findings: 0.
