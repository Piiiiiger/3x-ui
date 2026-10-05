# ========================================================
# Stage: Frontend (Vite)
# ========================================================
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
COPY internal/web/translation /src/internal/web/translation
RUN npm run build

# ========================================================
# Stage: Builder
# ========================================================
FROM golang:1.27-alpine AS builder
WORKDIR /app
ARG TARGETARCH

RUN apk --no-cache --update add \
  build-base \
  bash \
  gcc \
  curl \
  unzip

COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist

ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
RUN go build -ldflags "-w -s" -o build/x-ui main.go
RUN bash deploy/pigger-agent/package.sh "$(cat internal/config/version)" build/agent-releases
RUN ./DockerInit.sh "$TARGETARCH"

# ========================================================
# Stage: Final Image of 3x-ui
# ========================================================
FROM alpine
ENV TZ=Asia/Tehran
WORKDIR /app

RUN apk add --no-cache --update \
  ca-certificates \
  tzdata \
  bash \
  curl \
  openssl

COPY --from=builder /app/build/ /app/
COPY --from=builder /app/DockerEntrypoint.sh /app/
COPY --from=builder /app/x-ui.sh /usr/bin/x-ui
COPY --from=builder /app/internal/web/translation /app/internal/web/translation


RUN chmod +x \
  /app/DockerEntrypoint.sh \
  /app/x-ui \
  /usr/bin/x-ui

ENV XUI_IN_DOCKER="true"
ENV XUI_MAIN_FOLDER="/app"
ENV XUI_DB_TYPE=""
ENV XUI_DB_DSN=""
EXPOSE 2053
VOLUME [ "/etc/x-ui" ]
CMD [ "./x-ui" ]
ENTRYPOINT [ "/app/DockerEntrypoint.sh" ]
