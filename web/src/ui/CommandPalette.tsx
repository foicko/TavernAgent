// 命令面板原语：只负责"渲染 + 键盘 + 无障碍"，不知道任何业务。
//
// 挂载即打开（由外层决定何时渲染），命令清单由 lib/commandRegistry 装配后传进来。
// 依赖面：react 与 ../lib/commands（纯模块，不含 store）——叶子层约束见 scripts/check.mjs。
import { Fragment, useCallback, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { filterCommands, isCommandEnabled, sectionize, type Command } from "../lib/commands";
import "./CommandPalette.css";

export interface CommandPaletteProps {
  commands: Command[];
  onClose: () => void;
  /** 对话框的无障碍名。 */
  label?: string;
  placeholder?: string;
}

export function CommandPalette({
  commands,
  onClose,
  label = "命令面板",
  placeholder = "搜索命令：角色、故事、面板、设置…",
}: CommandPaletteProps) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const visible = useMemo(() => filterCommands(commands, query), [commands, query]);
  const enabledFlags = useMemo(() => visible.map(isCommandEnabled), [visible]);
  const sections = useMemo(() => sectionize(visible), [visible]);

  // 高亮位置在渲染时校正：查询变化、或在命令面板打开期间某个命令变得不可用（例如
  // 生成结束时"停止当前生成"消失）都不需要额外的同步 effect，因而也不会误重置。
  const firstEnabled = enabledFlags.findIndex(Boolean);
  const activePosition = enabledFlags[activeIndex] ? activeIndex : firstEnabled;
  const active = activePosition >= 0 ? visible[activePosition] : undefined;

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    // 可选调用：jsdom 与个别旧内核没有 scrollIntoView，缺了它也只影响"高亮跟着滚"这一件事。
    listRef.current?.querySelector<HTMLElement>(`[data-index="${activePosition}"]`)?.scrollIntoView?.({ block: "nearest" });
  }, [activePosition]);

  const moveBy = useCallback(
    (delta: number) => {
      const count = visible.length;
      if (count === 0 || firstEnabled < 0) return;
      let next = activePosition >= 0 ? activePosition : firstEnabled;
      for (let step = 0; step < count; step += 1) {
        next = (next + delta + count) % count;
        if (enabledFlags[next]) break;
      }
      setActiveIndex(next);
    },
    [activePosition, enabledFlags, firstEnabled, visible.length],
  );

  const jumpTo = useCallback(
    (index: number) => {
      if (index >= 0) setActiveIndex(index);
    },
    [],
  );

  const runCommand = useCallback(
    (command: Command | undefined) => {
      if (!command || !isCommandEnabled(command)) return;
      void command.run();
      onClose();
    },
    [onClose],
  );

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    switch (event.key) {
      case "Escape":
        // 面板是最上层：吃掉这次 Esc，既不关侧栏也不关下层弹窗。
        event.preventDefault();
        event.stopPropagation();
        onClose();
        return;
      case "Tab":
        // 焦点不逃逸：唯一可聚焦的是搜索框，列表用 aria-activedescendant 走虚拟焦点。
        event.preventDefault();
        return;
      case "ArrowDown":
        event.preventDefault();
        moveBy(1);
        return;
      case "ArrowUp":
        event.preventDefault();
        moveBy(-1);
        return;
      case "Home":
        event.preventDefault();
        jumpTo(firstEnabled);
        return;
      case "End":
        event.preventDefault();
        jumpTo(enabledFlags.lastIndexOf(true));
        return;
      case "Enter":
        event.preventDefault();
        runCommand(active);
        return;
      default:
        return;
    }
  };

  const optionId = (index: number) => `${listId}-option-${index}`;

  return (
    <div
      className="ui-command-palette__backdrop"
      data-testid="command-palette-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="ui-command-palette" role="dialog" aria-modal="true" aria-label={label} onKeyDown={handleKeyDown}>
        <div className="ui-command-palette__search">
          <svg className="ui-command-palette__search-icon" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
            <circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
            <path d="M10.5 10.5 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
          <input
            ref={inputRef}
            className="ui-command-palette__input"
            type="text"
            value={query}
            role="combobox"
            aria-expanded="true"
            aria-controls={listId}
            aria-activedescendant={active ? optionId(activePosition) : undefined}
            aria-label={label}
            autoComplete="off"
            spellCheck={false}
            placeholder={placeholder}
            onChange={(event) => setQuery(event.target.value)}
          />
          <kbd className="ui-command-palette__key">Esc</kbd>
        </div>

        <ul className="ui-command-palette__list" id={listId} role="listbox" aria-label={label} ref={listRef}>
          {visible.length === 0 ? (
            <li className="ui-command-palette__empty" role="presentation">
              没有匹配的命令，换个说法试试
            </li>
          ) : (
            sections.map((section) => (
              <Fragment key={section.group}>
                <li className="ui-command-palette__group" role="presentation">
                  {section.label}
                </li>
                {section.items.map(({ command, index }) => {
                  const disabled = !enabledFlags[index];
                  return (
                    <li
                      key={command.id}
                      id={optionId(index)}
                      role="option"
                      aria-selected={index === activePosition}
                      aria-disabled={disabled || undefined}
                      data-index={index}
                      data-testid={`command-${command.id}`}
                      className={[
                        "ui-command-palette__item",
                        index === activePosition ? "ui-command-palette__item--active" : "",
                        disabled ? "ui-command-palette__item--disabled" : "",
                      ]
                        .filter(Boolean)
                        .join(" ")}
                      onMouseEnter={() => {
                        if (!disabled) setActiveIndex(index);
                      }}
                      onMouseDown={(event) => {
                        event.preventDefault();
                        runCommand(command);
                      }}
                    >
                      <span className="ui-command-palette__title">{command.title}</span>
                      {command.hint ? <span className="ui-command-palette__hint">{command.hint}</span> : null}
                    </li>
                  );
                })}
              </Fragment>
            ))
          )}
        </ul>

        <div className="ui-command-palette__footer">
          <span>
            <kbd className="ui-command-palette__key">↑</kbd>
            <kbd className="ui-command-palette__key">↓</kbd> 选择
          </span>
          <span>
            <kbd className="ui-command-palette__key">Enter</kbd> 执行
          </span>
          <span>
            <kbd className="ui-command-palette__key">Esc</kbd> 关闭
          </span>
        </div>
      </div>
    </div>
  );
}
