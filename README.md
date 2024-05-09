# converge

converge is a file syncing utility designed to work on fast storage systems and
networks. It works very similarly to rsync in archive mode but runs concurrent
transfers and can multiplex traffic over multiple SSH connections.

## usage

converge only works in a "push" orientation where data is transfered from the
local running client to a remote SSH host:

```
$ converge dir host:dir --transfer-channels 2 --copy-threads 12
```

### argument usage

- `transfer-channels` controls the number of concurrent SSH connections to open
- `copy-threads` determines the number of concurrent copies to run, and must be
  divisible by the number of transfer channels (for load distribution)
- `split-threshold` can help when transfering many large files by splitting
  transfers for any files larger than this threshold (by default 10Gb)
- `split-concurrency` determines the number of concurrent copies to run per file
  split via `split-threshold`. This has no relation to `copy-threads`

## performance

The following example shows copying a directory with many small to medium sized
files from/to SSD storage over a 40Gbps LAN.

```
$ time ./converge a sol:b --transfer-channels 4 --copy-threads 24
44.202 total

$ time rsync -a --info=progress2 a sol:b
2:52.98 total
```

In other cases with more mixed file sizes converge can still outperform rsync
easily (much of this comes from the single ssh connection limitation in rsync).

```
$ time rsync -a test sol:test
33:42.95 total

$ time ./converge test sol:test --transfer-channels 4 --copy-threads 24
17:05.04 total
```
