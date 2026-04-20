# Judge Runtime Images

These Dockerfiles build the runtime images used by the executor so every language image includes `/usr/bin/time`.

## Images

- `judge-cpp:latest`
- `judge-java:latest`
- `judge-python:3.10`
- `judge-kotlin:17`

## Build all images

```powershell
powershell -ExecutionPolicy Bypass -File .\docker\build-images.ps1
```

## Build one image manually

```powershell
docker build -t judge-python:3.10 -f docker/python/Dockerfile .
docker build -t judge-cpp:latest -f docker/cpp/Dockerfile .
docker build -t judge-java:latest -f docker/java/Dockerfile .
docker build -t judge-kotlin:17 -f docker/kotlin/Dockerfile .
```

## Verify `time` exists

```powershell
docker run --rm judge-python:3.10 sh -lc "command -v /usr/bin/time"
docker run --rm judge-cpp:latest sh -lc "command -v /usr/bin/time"
docker run --rm judge-kotlin:17 sh -lc "command -v /usr/bin/time"
docker run --rm judge-java:latest sh -lc "command -v /usr/bin/time"
```
