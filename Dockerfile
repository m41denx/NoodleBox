FROM alpine
RUN apk add --no-cache tzdata
RUN mkdir /app /core
COPY NoodleBox /app
COPY amongus.db /app
CMD ["/app/NoodleBox"]