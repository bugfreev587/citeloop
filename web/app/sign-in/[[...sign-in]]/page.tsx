import { SignIn } from "@clerk/nextjs";

export default function SignInPage() {
  return (
    <main className="grid min-h-[100dvh] place-items-center bg-stone-100 px-4 py-8">
      <SignIn />
    </main>
  );
}
