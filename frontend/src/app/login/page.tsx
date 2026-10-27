import { GoogleSignIn } from "@/components/auth/GoogleSignIn";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

const GOOGLE_CLIENT_ID =
  process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID || "your-google-client-id";
const GOOGLE_REDIRECT_URI =
  process.env.NEXT_PUBLIC_GOOGLE_REDIRECT_URI ||
  "http://localhost:3000/auth/callback";

export default function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-950 px-4">
      <Card className="w-full max-w-md shadow-2xl">
        <CardHeader>
          <CardTitle className="text-2xl">Sign in to KubeFix Labs</CardTitle>
        </CardHeader>
        <CardContent className="py-6">
          <p className="text-sm text-gray-400 mb-6">
            Sign in with your Google account to start troubleshooting challenges. Free tier
            includes 10 challenge-start credits per account.
          </p>
          <GoogleSignIn
            clientId={GOOGLE_CLIENT_ID}
            redirectUri={GOOGLE_REDIRECT_URI}
          />
        </CardContent>
      </Card>
    </div>
  );
}

// Minimal link shim (kept inline to avoid an extra file)
const Link = ({ children }: { children: React.ReactNode }) => (
  <a className="text-brand-400 hover:underline">{children}</a>
);
