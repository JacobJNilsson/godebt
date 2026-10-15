# Corpus results

Every row is one public module at one pinned version, audited with no
configuration. The index is 0 to 100 and lower is better. The two points columns
hold the points that each dimension contributed to the index. Production counts
cover the measured production set only. A run of make corpus rewrites this file.

| module | version | index | complexity-erosion points | duplication points | production code lines | production functions | eroded functions | excess mass | clone groups | completeness |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| github.com/mattn/go-isatty | v0.0.20 | 2 | 2 | 0 | 142 | 15 | 1 | 5.0 | 0 | complete |
| github.com/pkg/errors | v0.9.1 | 0 | 0 | 0 | 277 | 32 | 0 | 0.0 | 0 | complete |
| github.com/fatih/color | v1.17.0 | 13 | 0 | 13 | 392 | 83 | 0 | 0.0 | 3 | complete |
| github.com/google/uuid | v1.6.0 | 17 | 11 | 6 | 805 | 72 | 3 | 92.6 | 1 | complete |
| github.com/go-chi/chi/v5 | v5.1.0 | 21 | 17 | 4 | 2655 | 234 | 5 | 385.8 | 2 | complete |
| github.com/gorilla/websocket | v1.5.3 | 38 | 38 | 0 | 2938 | 165 | 9 | 1641.6 | 0 | complete |
| github.com/stretchr/testify | v1.9.0 | 54 | 36 | 18 | 3180 | 205 | 9 | 1757.2 | 11 | complete |
| go.uber.org/zap | v1.27.0 | 26 | 14 | 12 | 4897 | 663 | 6 | 356.6 | 10 | complete |
| github.com/rs/zerolog | v1.33.0 | 55 | 32 | 23 | 5611 | 568 | 13 | 1761.5 | 28 | complete |
| gopkg.in/yaml.v3 | v3.0.1 | 68 | 51 | 17 | 7710 | 302 | 57 | 8954.1 | 22 | complete |
| github.com/prometheus/client_golang | v1.19.1 | 43 | 24 | 19 | 9330 | 657 | 23 | 1380.8 | 28 | complete |
| github.com/valyala/fasthttp | v1.55.0 | 53 | 34 | 19 | 14392 | 1172 | 56 | 4552.4 | 35 | complete |
| k8s.io/apimachinery | v0.31.0 | 49 | 30 | 19 | 29518 | 2435 | 87 | 5536.1 | 72 | complete |
| google.golang.org/grpc | v1.65.0 | 49 | 31 | 18 | 50191 | 3644 | 129 | 8360.8 | 87 | complete |
| github.com/prometheus/prometheus | v0.54.0 | 63 | 39 | 24 | 75973 | 4660 | 282 | 26475.6 | 256 | complete |

Over the 7 modules above 5000 production lines, the Spearman rank correlation of the index is -0.16 with production code lines and +0.85 with debt per thousand production lines. Debt is eroded functions plus clone groups.
