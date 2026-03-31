import { mount } from "svelte";
import Root from "./Root.svelte";
import "./app.css";

const target = document.getElementById("app");

if (!target) {
  throw new Error("Root element 'app' not found. Cannot mount application.");
}

mount(Root, { target });
