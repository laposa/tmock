FROM node:18-bullseye

RUN apt-get -y update && apt-get -y upgrade

WORKDIR /usr/src

COPY . ./
RUN corepack enable \
    && npm install \
    && npm run build

USER node
CMD node dist/src/main
