# osprey - A log scanning program.

## Get osprey

```shell script
$ go get osprey
```

## What is osprey?

Osprey is a log scanning program, but it is more than just log scanning. When it detects 
a new error log, it reports the error as a Github issue automatically. 

## Features

- Use Github API V3 to create Github issues.
- Target services (whose log files will be scanned later) can be easily defined in a config file (`osprey.yml`).
- Docker container deployment friendly.

## How it works?

When osprey starts:
- it first loads all the target services from `osprey.yml`;
- then, osprey creates a monitor file (with extension of `.igu`) for each service;
- it creates a worker pool based on the number of services to be monitored;
- it starts scanning tasks using those workers.

When it scans:
- it first read the `anchor` point (the last location in log file) from `.igu` file;
- then it start scanning from the anchor point, check out if there are new error logs;
- when new error logs are founded, Github issues will be created and submitted;
- the anchor value is updated.

## An example of `osprey.yml` file.

```yaml
interval: 5
max_workers: 20
igu_file_path: /tmp/igu
services:
  apple:
    mode: local
    location: /tmp/log/apple.log
    repo_owner: owner
    repo_name: osprey
  orange:
    mode: local
    location: /tmp/log/orange.log
    repo_owner: owner
    repo_name: osprey
```

Here:
- interval - the interval between two consecutive scans;
- max_workers - maximal number of workers;
- igu_file_path - path to store all the `.igu` files;
- apple、orange - target services, for each service:
    - mode - log file reading mode
        - local - read from local volume（e.g. local file system, shared docker volumes)
        - remote (NOT SUPPORT YET)
    - location - full path of the log file；
    - repo_owner - the owner of the repository where issues will be submitted to;
    - repo_name - the name of the repository where issues will be submitted to;

## Run it

### Run As Stand-alone App.

You can change where you want 
```shell script
$ make run
```

### Run In Docker-container Environment

Assume we have two services: apple and orange. We can run osprey along with those 
two services in docker-container environment. You must define a shared volume to allow 
osprey to access the log files of those services. 

Here is the example of a docker-compose config file:

```dockerfile
version: '3'

services:
  apple:
    build:
      context: service_apple
    volumes:
      - log-volume:/tmp/log/

  orange:
    build:
      context: service_orange
    volumes:
      - log-volume:/tmp/log/

  osprey:
    build:
      context: ../

volumes:
  log-volume:
```

You need create the `log-volume` before start:
```shell script
$ docker volume create log-volume
```

## TODO
- Read log file remotely (e.g., nfs, a volume on a remote host).
