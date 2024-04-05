# Library Management System

## Getting Started

The fastest way to get started with the Library Management System is to use
Docker. If you don't have Docker installed, you can download it from
[here](https://www.docker.com/products/docker-desktop).

Once you have Docker installed, you can run the following commands to build the
container image and execute the application:

```bash
docker build -t admtnnr/library .
docker run -i --rm admtnnr/library - < testdata/all_commands.jsonl
```
