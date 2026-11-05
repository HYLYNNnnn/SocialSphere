import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import Login from "./Login";
import Register from "./Register";
import Collection from "./Collection";
import Landing from "./Landing";

export default function Main({ session, handleLoggedIn }) {
  const home = session ? "/create" : "/login";
  return (
    <main className="main">
      <Routes>
        <Route path="/" element={<Navigate to={home} replace />} />
        <Route path="/login" element={session ? <Navigate to="/create" replace /> : <Login handleLoggedIn={handleLoggedIn} />} />
        <Route path="/register" element={session ? <Navigate to="/create" replace /> : <Register />} />
        <Route path="/create" element={session ? <Landing /> : <Navigate to="/login" replace />} />
        <Route path="/collection" element={session ? <Collection username={session.username} /> : <Navigate to="/login" replace />} />
        <Route path="*" element={<Navigate to={home} replace />} />
      </Routes>
    </main>
  );
}
