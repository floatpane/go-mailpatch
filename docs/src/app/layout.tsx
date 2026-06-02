import type { Metadata } from "next";
import "./globals.css";
import "highlight.js/styles/github-dark.css";

export const metadata: Metadata = {
	title: "go-mailpatch",
	description:
		"Parse git format-patch emails in Go. Commit metadata, [PATCH n/m] series, unified diffs, and diffstat — from a single message or a whole mbox, zero dependencies.",
};

export default function RootLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<html lang="en">
			<body>
				<header className="site-header">
					<a href="/" className="brand">
						go-mailpatch
					</a>
					<nav>
						<a href="https://github.com/floatpane/go-mailpatch">GitHub</a>
					</nav>
				</header>
				<main>{children}</main>
			</body>
		</html>
	);
}

export const viewport = { width: "device-width", initialScale: 1 };
