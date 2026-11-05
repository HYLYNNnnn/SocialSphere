import React, { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Spin, message } from "antd";
import ResponsiveAppBar from "./ResponsiveAppBar";
import Main from "./Main";
import { clearSession, getSession, verifySession, SESSION_EXPIRED_EVENT } from "../api";
import { TOKEN_KEY } from "../constants";

function App() {
  const [session, setSession] = useState(getSession);
  const [checking, setChecking] = useState(() => !!getSession());
  const [sessionError, setSessionError] = useState("");
  const restoration = useRef(0);
  const restoreSession = useCallback(async () => {
    const version = ++restoration.current;
    setSessionError("");
    if (!getSession()) { setSession(null); setChecking(false); return; }
    setChecking(true);
    try {
      const verified = await verifySession();
      if (version === restoration.current) setSession(verified);
    } catch (error) {
      if (version === restoration.current) {
        if (error.status === 401) { clearSession(); setSession(null); }
        else setSessionError(error.message);
      }
    } finally {
      if (version === restoration.current) setChecking(false);
    }
  }, []);

  useEffect(() => {
    restoreSession();
    const onExpired = () => {
      setSession(null);
      message.info("Your session has ended. Please sign in again.");
    };
    const onStorage = (event) => {
      if (event.key === TOKEN_KEY || event.key === null) restoreSession();
    };
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    window.addEventListener("storage", onStorage);
    return () => {
      restoration.current += 1;
      window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
      window.removeEventListener("storage", onStorage);
    };
  }, [restoreSession]);

  useEffect(() => {
    if (!session) return;
    const expire = () => {
      restoration.current += 1;
      clearSession(); setSession(null); setChecking(false);
      message.info("Your session has ended. Please sign in again.");
    };
    const timer = setTimeout(expire, Math.min(Math.max(session.expiresAt - Date.now(), 0), 2147483647));
    return () => clearTimeout(timer);
  }, [session]);

  const logout = () => {
    restoration.current += 1;
    clearSession(); setSession(null); setChecking(false); setSessionError("");
  };
  return (
    <div className="App">
      <ResponsiveAppBar isLoggedIn={!!session && !checking && !sessionError} username={session?.username} handleLogout={logout} />
      {checking ? <div role="status" className="session-status"><Spin tip="Checking your account…" /></div>
        : sessionError ? <Alert type="error" message="Unable to verify your session" description={sessionError}
          action={<Button onClick={restoreSession}>Retry</Button>} />
        : <Main session={session} handleLoggedIn={setSession} />}
    </div>
  );
}
export default App;
