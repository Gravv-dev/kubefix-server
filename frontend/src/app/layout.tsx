import type { Metadata } from "next";
import { Inter } from "next/font/google";
import { Roboto_Mono } from "next/font/google";
import "./globals.css";

const inter = Inter({ subsets: ["latin"], variable: "--font-inter" });
const robotoMono = Roboto_Mono({ subsets: ["latin"], variable: "--font-roboto-mono" });

export const metadata: Metadata = {
  title: "KubeFix Labs | Docker & Kubernetes Troubleshooting",
  description: "Practice real-world Docker and Kubernetes troubleshooting in isolated, disposable environments.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${inter.variable} ${robotoMono.variable} bg-gray-900 text-gray-100 min-h-screen antialiased`}>
        {children}
      </body>
    </html>
  );
}
