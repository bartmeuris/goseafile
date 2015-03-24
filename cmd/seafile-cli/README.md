# Seafile CLI tool

## Useage

Run `seafile-cli -h` to see the commandline options.


## Scripting

There is a rudamentary form of scripting available, see the [example script](examples/script.sea) for reference.

Any commandline parameter passed after `seafile-cli` parameters will be available as `$1`, `$2`, ... (or in `${1}` notation). Environment variables are also available in the same format.

