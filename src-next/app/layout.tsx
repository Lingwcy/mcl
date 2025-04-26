import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { UIProvider } from "./context/UIContext";
import { PathProvider } from "./context/PathContext"; // Make sure this import is correct

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "Miao Craft Launcher",
  description: "A modern Minecraft launcher",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" data-theme="light">
      <body className={inter.className}>
        <PathProvider>
          <UIProvider>
            {children}
          </UIProvider>
        </PathProvider>
      </body>
    </html>
  );
}
