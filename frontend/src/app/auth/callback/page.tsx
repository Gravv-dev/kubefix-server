"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";

export default function GoogleCallbackPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const started = useRef(false);

  useEffect(() => {
    if (started.current) {
      return;
    }
    started.current = true;

    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const receivedState = params.get("state");
    const expectedState = sessionStorage.getItem("google_oauth_state");
    sessionStorage.removeItem("google_oauth_state");

    if (params.has("error")) {
      setError(params.get("error_description") || "Google sign-in was cancelled.");
      return;
    }

    if (!code || !receivedState || !expectedState || receivedState !== expectedState) {
      setError("Google sign-in could not be verified. Please try again.");
      return;
    }

    api
      .post("/api/auth/google", { code })
      .then(() => {
        router.replace("/");
        router.refresh();
      })
      .catch((cause: unknown) => {
        setError(
          cause instanceof Error ? cause.message : "Google sign-in failed. Please try again."
        );
      });
  }, [router]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-950 px-4">
      <Card className="w-full max-w-md shadow-2xl">
        <CardHeader>
          <CardTitle className="text-2xl">
            {error ? "Sign-in failed" : "Signing you in"}
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          {error ? (
            <>
              <p role="alert" className="mb-4 text-sm text-red-400">
                {error}
              </p>
              <Link href="/login" className="text-sm text-brand-400 hover:underline">
                Back to sign in
              </Link>
            </>
          ) : (
            <p role="status" className="text-sm text-gray-400">
              Please wait while we complete Google sign-in…
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
