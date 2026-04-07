import { apiRequest } from "./client";

export function getEvents() {
  return apiRequest("/events");
}
