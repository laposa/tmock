export type AppConfig = {
  baseUrl: string;
  apiEndpoint: string;
  buildTimestamp: string;

  version: {
    major: number;
    minor: number;
    build: string;
    timestamp: string;
  };
};
