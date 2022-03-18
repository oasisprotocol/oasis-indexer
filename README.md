# Oasis Block Indexer

A block indexer for the Oasis Network.

## Docker Environment

You can create and run a Dockerized version of the Oasis Indexer as follows.

**Build**
```sh
$ docker build --tag oasislabs/oasis-indexer:dev --file docker/indexer.Dockerfile .
```

**Run**
```sh
$ alias oasis-indexer="docker run oasislabs/oasis-indexer:dev"
$ oasis-indexer --help
```
