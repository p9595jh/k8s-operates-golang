FROM golang:1.25-alpine AS base

WORKDIR /work

COPY . .

RUN go build -o app .

FROM scratch

WORKDIR /

COPY --from=base /work/app .
COPY --from=base /work/docs .

EXPOSE 8070

ARG env

CMD ["./app"]
