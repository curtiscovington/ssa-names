# ssa-names CLI

This repository includes a Go command-line tool that works with the `namesbystate` dataset. The dataset is compiled into the binary, so no additional files are required when running the tool.

## Build

```sh
go build ./cmd/names
```

## Commands

### Top (default)

```sh
./names -state CA -year 2015 -gender F -top 5
./names -year 2018-2020 -gender F -top 5
./names -state CA -year 2015 -gender F -name Olivia
```

Flags:

- `-state`: optional two-letter state abbreviation (omit for national totals).
- `-year`: optional year filter (comma-separated list or `start-end` range; `0` or empty means all years).
- `-gender`: optional gender filter (`M`, `F`, or leave empty).
- `-top`: number of names to display (minimum 1).
- `-name`: specific name to report rank for (requires `-year`).

The command prints the most popular names for the chosen filters. Omitting `-state` aggregates results across the entire United States. When `-year` is blank or `0`, the command considers the full dataset; otherwise it accepts individual years (`2019`), comma-separated lists (`2018,2020,2022`), and inclusive ranges (`2015-2019`). When `-name` is provided, it additionally reports that name's rank and occurrence count for the same filters.

Sample run:

```sh
go run ./cmd/names --state CA --year 2019 --gender F --top 3
```

```text
Top 3 names in CA for 2019 (F):

Rank  Name    Count
1     Olivia  2610
2     Emma    2402
3     Mia     2366
```

### Trend

```sh
./names trend -name Ashley -state CA -gender F
./names trend -name Michael -gender M
./names trend -names Emily,Ashley,Jessica -state CA -gender F --plot --metric rank
./names trend -name Ashley -state CA -gender F --svg ashley_ca.svg --svg-width 640 --svg-height 360
```

Flags:

- `-name`: single name to track.
- `-names`: comma-separated list of names for side-by-side comparison.
- `-state`: optional two-letter state abbreviation (omit for nationwide totals).
- `-gender`: optional gender filter (`M`, `F`, or leave empty).
- `--plot`: render a simple ASCII sparkline for the chosen metric.
- `--metric`: plotting metric (`rank`, `count`, or `share`; default `rank`).
- `--width` / `--height`: dimensions for the ASCII plot when `--plot` is enabled.
- `--svg`: write an SVG chart to the provided path.
- `--svg-width` / `--svg-height`: pixel dimensions for the SVG output (defaults 800×400).

The trend subcommand prints a chronological table of rank and count for each requested name. When `--plot` is used, it also renders an ASCII visualization of how the selected metric evolves over time.

Sample run:

```sh
go run ./cmd/names trend --name Ava --state HI --gender F --plot --metric rank --width 8
```

```text
Trend for Ava (F, HI):

Year  Ava Rank  Ava Count
1910  -         -
1911  -         -
...
2023  6         30
2024  8         31

Plot (metric=rank)
      ██
...
Legend: █ Ava
(higher = better rank)
```

*Output truncated for brevity.*

### Generate

```sh
./names generate --state CA --year 2019 --gender F --count 5 --seed 42
```

Flags:

- `--state`: optional two-letter state abbreviation (omit for national totals).
- `--year`: optional year filter (`0` means all years).
- `--gender`: optional gender filter (`M`, `F`, or leave empty).
- `--count`: number of random names to generate (default `1`).
- `--seed`: optional RNG seed for reproducible results.
- `--format`: output format (`table`, `json`, or `csv`).

The generate subcommand samples names according to their historical popularity, producing one or many picks that follow the dataset's probability distribution.

Sample run:

```sh
go run ./cmd/names generate --state CA --year 2019 --gender F --count 3 --seed 7
```

```text
Generated 3 names for CA in 2019 (F)

Pick  Name     DatasetCount  Chance
1     Anya     65            0.04%
2     Emily    1471          0.80%
3     Cecilia  167           0.09%
```

## Dataset Source

This project uses the United States Social Security Administration (SSA) baby names dataset — State‑specific data — available at the [SSA Baby Names by State download page](https://www.ssa.gov/oact/babynames/limits.html).

The dataset lists, for each U.S. state and year, the number of babies given each reported name. Each record provides the state abbreviation, gender, year, baby name, and occurrence count, letting us analyze popularity trends and probability distributions over time and geography.

Please see the SSA’s documentation for details and terms. An accompanying `StateReadMe.pdf` from the SSA is included at `data/namesbystate/StateReadMe.pdf` for reference.
