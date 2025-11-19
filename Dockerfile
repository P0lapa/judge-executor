FROM eclipse-temurin:17-jdk

RUN apt-get update && \
    apt-get install -y wget unzip && \
    wget https://github.com/JetBrains/kotlin/releases/download/v1.9.24/kotlin-compiler-1.9.24.zip && \
    unzip kotlin-compiler-1.9.24.zip -d /opt/kotlin && \
    rm kotlin-compiler-1.9.24.zip && \
    apt-get clean

ENV PATH="/opt/kotlin/kotlinc/bin:${PATH}"