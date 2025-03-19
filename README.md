# medialocator

Medialocator will print the file location of movies and TV shows in a [Plex](https://www.plex.tv/) database that have the given labels.

## Build

Just run `go build .`

## Run

The most useful call is something like this:

```sh
./medialocator \
  -tags preserve,classic \
  -plex-db /path/to/com.plexapp.plugins.library.db \
  -output-file media.list
```

Where **preserve** and **classic** are two comma-separated tags to search for. If `-output-file` is not specified, the list will be printed to standard output.

## Copyright

Davide Alberani <da@mimante.net> 2025.

Released under the Apache 2.0 license.

The author is in no way associated with Plex Inc.
