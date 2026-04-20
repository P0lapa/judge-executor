$ErrorActionPreference = "Stop"

$images = @(
    @{ Tag = "judge-cpp:latest"; Dockerfile = "docker/cpp/Dockerfile" },
    @{ Tag = "judge-java:latest"; Dockerfile = "docker/java/Dockerfile" },
    @{ Tag = "judge-python:3.10"; Dockerfile = "docker/python/Dockerfile" },
    @{ Tag = "judge-kotlin:17"; Dockerfile = "docker/kotlin/Dockerfile" }
)

foreach ($image in $images) {
    Write-Host "Building $($image.Tag) from $($image.Dockerfile)..."
    docker build -t $image.Tag -f $image.Dockerfile .
}
