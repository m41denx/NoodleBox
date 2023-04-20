FROM alpine
EXPOSE 8080
RUN apk add --no-cache tzdata
RUN mkdir /app /core
COPY NoodleBox /app
CMD ["/app/NoodleBox"]