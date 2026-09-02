import { cn } from "@/lib/utils";

/** macOS Terminal chrome — traffic lights + title bar around a dark pane. */
export default function MacTerm({
    title = "Terminal",
    actions,
    children,
    className,
    bodyClassName,
    bodyRef,
    bodyProps,
}) {
    return (
        <div className={cn("macos-term", className)}>
            <div className="macos-term-chrome flex items-center gap-3 px-3.5 py-2.5">
                <div className="flex shrink-0 items-center gap-[7px]" aria-hidden>
                    <span className="macos-term-dot macos-term-dot-close" />
                    <span className="macos-term-dot macos-term-dot-min" />
                    <span className="macos-term-dot macos-term-dot-max" />
                </div>
                <div className="min-w-0 flex-1 text-center">
                    <span className="macos-term-title mono truncate text-[11px]">{title}</span>
                </div>
                <div className="flex min-w-[4.5rem] shrink-0 items-center justify-end gap-1">
                    {actions}
                </div>
            </div>
            <div
                ref={bodyRef}
                className={cn("macos-term-body mono", bodyClassName)}
                {...bodyProps}
            >
                {children}
            </div>
        </div>
    );
}
