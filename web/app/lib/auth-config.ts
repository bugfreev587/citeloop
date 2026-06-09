export const clerkServerAuthConfigured = Boolean(
  process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY && process.env.CLERK_SECRET_KEY
);

const deploymentEnv = process.env.VERCEL_ENV;

export const allowUnconfiguredClerkBypass = deploymentEnv
  ? deploymentEnv !== "production"
  : process.env.NODE_ENV !== "production";

export function requireConfiguredClerk() {
  if (!clerkServerAuthConfigured && !allowUnconfiguredClerkBypass) {
    throw new Error("Clerk server authentication must be configured in production.");
  }
}
