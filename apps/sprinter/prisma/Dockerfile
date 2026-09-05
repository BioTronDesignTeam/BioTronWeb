# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

CMD ["npm", "run", "deploy"]
