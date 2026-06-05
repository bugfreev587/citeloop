import { SignUp } from "@clerk/nextjs";

export default function SignUpPage() {
  return (
    <main className="grid min-h-[100dvh] place-items-center bg-stone-100 px-4 py-8">
      <SignUp />
    </main>
  );
}
