FROM node:18-bullseye

RUN apt-get -y update && apt-get -y upgrade

WORKDIR /usr/src

COPY . ./
RUN corepack enable \
    && yarn install \
    && yarn build

USER node
CMD node dist/src/main
