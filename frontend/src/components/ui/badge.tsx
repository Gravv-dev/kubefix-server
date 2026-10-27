import * as React from "react";
import { cn } from "./cn";

export const Badge = React.forwardRef<
  HTMLSpanElement,
  React.HTMLAttributes<HTMLSpanElement> & { variant?: "default" | "brand" | "destructive" }
>(({ className, variant = "default", ...props }, ref) => {
  const variants: Record<string, string> = {
    default:
      "border-transparent bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-gray-100",
    brand: "bg-brand-600 text-white",
    destructive:
      "border-transparent bg-red-100 text-red-900 dark:bg-red-900 dark:text-red-100",
  };
  return (
    <span
      ref={ref}
      className={cn(
        "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none",
        variants[variant],
        className
      )}
      {...props}
    />
  );
});
Badge.displayName = "Badge";
