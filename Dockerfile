FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go build -o /out/newapi-video-wrapper .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/newapi-video-wrapper /app/newapi-video-wrapper
ENV PORT=18788
EXPOSE 18788
VOLUME ["/app/data"]
CMD ["/app/newapi-video-wrapper"]
