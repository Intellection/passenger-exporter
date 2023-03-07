ARG GOLANG_VERSION="1.20.1"
ARG DEBIAN_VERSION="bullseye-20230227-slim"

ARG BUILDER_IMAGE="golang:${GOLANG_VERSION}"
ARG RUNNER_IMAGE="debian:${DEBIAN_VERSION}"

FROM ${BUILDER_IMAGE} AS builder

WORKDIR /opt/app

# Install build tools
RUN go install github.com/prometheus/promu@v0.14.0

# Add source files and build
ADD . ./
RUN make

FROM ${RUNNER_IMAGE}

RUN apt-get update -y && \
    apt-get install -y \
      bash \
      ca-certificates \
      gnupg && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

ARG APP_USER="exporter"
ENV APP_HOME="/opt/app"

WORKDIR ${APP_HOME}

# Create user
RUN groupadd -g 9999 ${APP_USER} && \
    useradd --system --create-home -u 9999 -g 9999 ${APP_USER}

# Passenger
ARG PASSENGER_VERSION="6.0.17"
ARG PASSENGER_PKG="1:${PASSENGER_VERSION}-1~bullseye1"
RUN apt-key adv --no-tty --keyserver hkps://keyserver.ubuntu.com --recv-keys 561F9B9CAC40B2F7 && \
    echo 'deb https://oss-binaries.phusionpassenger.com/apt/passenger bullseye main' > /etc/apt/sources.list.d/passenger.list && \
    apt-get update -y && \
    apt-get install -y passenger=${PASSENGER_PKG} && \
    passenger-config validate-install --auto && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

# Copy files from builder
COPY --from=builder --chown=${APP_USER}:${APP_USER} ${APP_HOME}/bin/ ./bin/

# Run as user
USER ${APP_USER}:${APP_USER}

ENTRYPOINT ["./bin/passenger-exporter"]
