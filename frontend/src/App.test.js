import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import App from "./components/App";
import { clearSession, getSession, verifySession } from "./api";
import { TOKEN_KEY } from "./constants";

jest.mock("./api", () => ({ clearSession: jest.fn(), getSession: jest.fn(), verifySession: jest.fn(), SESSION_EXPIRED_EVENT: "test:session-expired" }));
jest.mock("./components/Main", () => {
  const React = require("react");
  return function Main({ session }) { return React.createElement("div", null, session ? `Signed in: ${session.username}` : "Sign-in screen"); };
});
jest.mock("./components/ResponsiveAppBar", () => {
  const React = require("react");
  return function Header({ isLoggedIn, handleLogout }) { return isLoggedIn ? React.createElement("button", { onClick: handleLogout }, "Log out") : null; };
});
jest.mock("antd", () => {
  const React = require("react");
  return {
    Alert: ({ message, description, action }) => React.createElement("div", { role: "alert" }, message, description, action),
    Button: ({ onClick, children }) => React.createElement("button", { onClick }, children),
    Spin: () => React.createElement("span", null, "Checking account"),
    message: { info: jest.fn() },
  };
});

const session = { token: "test-token", username: "alice", expiresAt: Date.now() + 3600000 };
beforeEach(() => { getSession.mockReturnValue(null); verifySession.mockResolvedValue(null); });

test("starts signed out when there is no saved token", () => {
  render(<App />);
  expect(screen.getByText("Sign-in screen")).toBeInTheDocument();
  expect(verifySession).not.toHaveBeenCalled();
});

test("verifies a restored account then supports logout", async () => {
  getSession.mockReturnValue(session); verifySession.mockResolvedValue(session);
  render(<App />);
  expect(await screen.findByText("Signed in: alice")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Log out" }));
  expect(clearSession).toHaveBeenCalled();
  expect(screen.getByText("Sign-in screen")).toBeInTheDocument();
});

test("shows a retry action when the server cannot verify the saved session", async () => {
  getSession.mockReturnValue(session);
  verifySession.mockRejectedValueOnce(new Error("Server unavailable")).mockResolvedValueOnce(session);
  render(<App />);
  expect(await screen.findByRole("alert")).toHaveTextContent("Server unavailable");
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(await screen.findByText("Signed in: alice")).toBeInTheDocument();
});

test("an invalid server-side session returns to sign-in", async () => {
  getSession.mockReturnValue(session); verifySession.mockRejectedValue({ status: 401, message: "Expired" });
  render(<App />);
  await waitFor(() => expect(screen.getByText("Sign-in screen")).toBeInTheDocument());
  expect(clearSession).toHaveBeenCalled();
});

test("a late restore response cannot undo logout in another tab", async () => {
  let resolve;
  const pending = new Promise((done) => { resolve = done; });
  getSession.mockReturnValue(session); verifySession.mockReturnValue(pending);
  render(<App />);
  getSession.mockReturnValue(null);
  act(() => window.dispatchEvent(new StorageEvent("storage", { key: TOKEN_KEY })));
  await act(async () => { resolve(session); await pending; });
  expect(screen.getByText("Sign-in screen")).toBeInTheDocument();
});
