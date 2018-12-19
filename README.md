# hackathon-2018

## build

### requierement

* install [go](https://golang.org/)
* install [go dep command](https://github.com/golang/dep)

### compilation

```
$ cd $PROJECT
$ dep ensure
$ go build -o scrap .
```

## run

```
$ ./scrap "foo bar"
$ DEVICE="mobile" ./scrap "foo" # with mobile user agent
$ MODE="prod" ./scrap "foo" # export metrics to csv
```
