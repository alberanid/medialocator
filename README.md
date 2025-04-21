# medialocator

Medialocator will print the file location of movies and TV shows in a [Plex](https://www.plex.tv/) database that have the given labels. This may be useful, for example, to get a list of title to backup.

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

It's also possible to get the complete list of all media parts with the `-list-all` option.

If you want to list all media items that have no associated tag, use the `-no-tags` option.

To filter by one or more library, use the `-libraries Comma,Separated,List,Of,Libraries` argument.

## Copyright

Davide Alberani <da@mimante.net> 2025.

Released under the Apache 2.0 license.

The author is in no way associated with Plex Inc.
