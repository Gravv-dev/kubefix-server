"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

export type GoogleSignInProps = {
  onSuccess?: () => void;
  clientId: string;
  redirectUri: string;
  scope?: string;
};

export function GoogleSignIn({
  onSuccess,
  clientId,
  redirectUri,
  scope = "openid email profile",
}: GoogleSignInProps) {
  const [loading, setLoading] = useState(false);

  const handleSignIn = () => {
    setLoading(true);
    const params = new URLSearchParams({
      client_id: clientId,
      redirect_uri: redirectUri,
      response_type: "code",
      scope,
      access_type: "offline",
      prompt: "consent",
    });
    const url = `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`;
    // Full-page redirect to Google; Google posts back to our callback with ?code=.
    window.location.href = url;
  };

  return (
    <Button
      variant="brand"
      onClick={handleSignIn}
      disabled={loading}
      className="w-full flex items-center justify-center gap-2"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden="true"
      >
        <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.77 3.28-8.09" />
        <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23" />
        <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 9.99 1 13.72 1 17.71c0 3.99 3.08 7.28 7.07 7.28h2.84v-7.07zM12 4.93c-1.48 0-2.82.4-3.93 1.06l-1.17-1.17C8.18 3.76 10.06 2 12 2c4.71 0 8.48 3.45 9.4 8.11h2.77c-.6-5.15-4.85-9.18-10.17-9.18" />
        <path d="M12 20.34c-3.53 0-6.52-2.09-7.65-5.13H2.18C2.9 20.11 7.14 23 12 23c5.3 0 9.56-3.82 10.29-8.89h-2.84c-.88 2.23-3.08 3.89-5.61 3.89" />
      </svg>
      Sign in with Google
    </Button>
  );
}
