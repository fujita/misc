You can specify the number of clients:

```
$ docker-compose -f docker-compose-gobgp.yml up --scale client=2
```

You can use RustyBGP as follows:

```
$ docker-compose -f docker-compose-rusty.yml up --scale client=2
```
