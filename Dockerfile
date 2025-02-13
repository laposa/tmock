FROM node:22-bookworm

RUN apt-get -y update && apt-get -y upgrade

WORKDIR /usr/src/app
COPY app ./
RUN npm install \
    && npm run build 

WORKDIR /usr/src/api
COPY api ./
RUN npm install \
    && npm run build \
    && cp -r ../app/dist ./public

# fix permissions
RUN chown -R node:node /usr/src/api/public

USER node
CMD node dist/src/main
