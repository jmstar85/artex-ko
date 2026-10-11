import type { ReactNode } from "react";

import { LanguageSwitcher } from "@/components/language-switcher";

export default function AuthLayout({ children }: { readonly children: ReactNode }) {
  return (
    <>
      <div className="fixed top-4 right-4 z-50">
        <LanguageSwitcher />
      </div>
      {children}
    </>
  );
}
