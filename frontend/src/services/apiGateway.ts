import axios from "axios";

const API_BASE =
  typeof import.meta !== "undefined" && import.meta.env?.VITE_API_URL
    ? (import.meta.env.VITE_API_URL as string).replace(/\/$/, "")
    : "https://uncomfortably-unshut-jaclyn.ngrok-free.dev";

export const metricsGateway = axios.create({
  baseURL: API_BASE,
  headers: {
    Accept: "application/json",
  },
});
