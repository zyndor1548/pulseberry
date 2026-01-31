import axios from "axios";

const API_BASE =
  typeof import.meta !== "undefined" && import.meta.env?.VITE_API_URL
    ? (import.meta.env.VITE_API_URL as string).replace(/\/$/, "")
    : "https://uncomfortably-unshut-jaclyn.ngrok-free.dev/";

/** In dev, use empty baseURL so requests hit Vite proxy (same-origin, no CORS). */
const baseURL = import.meta.env.DEV ? "" : API_BASE;

export const metricsGateway = axios.create({
  method: "GET",
  baseURL,
  headers: {
    Accept: "application/json",
  },
});
