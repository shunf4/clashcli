## Clashcli

A simple command-line client for [Clash](https://github.com/Dreamacro/clash).

Interacts with Clash by its REST API.

- Select nodes for groups
- Run latency test for a node

#### Help

```
$ clashcli --help
Usage of clashcli:

Specify environment variables to control which Clash instance to control and which groups to select from.
Environment variables will be overridden by command line arguments, flags and options.

    CLASH_PORT          Clash external controller port. If not specified,
                        9090, 9091, 19090, 19091 will be tried sequen-
                        tially.
    CLASH_ADDR          Clash external controller address. If not
                        specified, defaults to 127.0.0.1.
    CLASH_SCHEME        Clash external controller scheme. If not
                        specified, defaults to http.
    CLASH_GROUPS        Which groups to select from. Can be group names
                        or group indexes (starts from 0), separated by
                        commas. E.g. "My Proxy,Video Media,3".
    CLASH_TEST_URL      Delay test URL. Defaults to
                        connectivitycheck.gstatic.com/generate_204 .

Command line:
    clashcli [-h|--help]
    clashcli [-p <port>] [-a <addr>] [-u <url>] [-e <scheme>] [-s] [-t]
            [<Group1> [<Group2> [<G3> ...]]]

  -a string
        Clash external controller address
  -e string
        Clash external controller scheme
  -p int
        Clash external controller port (default -1)
  -s    (Select) Use node select feature. This is the default feature
  -t    (delay Test) Use delay test feature. You can specify only 1 proxy group in this case
  -u string
        Delay test URL
```

#### License

Until otherwise noted, all of clashcli source files are distributed under the terms of MIT License in LICENSE file.
