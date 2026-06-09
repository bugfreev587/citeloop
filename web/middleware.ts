import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { allowUnconfiguredClerkBypass, clerkServerAuthConfigured } from "./app/lib/auth-config";

const isPublicRoute = createRouteMatcher(["/", "/docs(.*)", "/sign-in(.*)", "/sign-up(.*)"]);

const protectedMiddleware = clerkMiddleware(async (auth, req) => {
  if (!isPublicRoute(req)) {
    await auth.protect();
  }
});

function unconfiguredMiddleware(_req: NextRequest) {
  if (!allowUnconfiguredClerkBypass) {
    return new NextResponse("Clerk server authentication is not configured.", { status: 503 });
  }
  return NextResponse.next();
}

export default clerkServerAuthConfigured ? protectedMiddleware : unconfiguredMiddleware;

export const config = {
  matcher: [
    "/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    "/(api|trpc)(.*)",
    "/__clerk/(.*)",
  ],
};
