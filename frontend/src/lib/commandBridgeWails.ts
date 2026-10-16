import {
  type CommandBridgeOwner,
  type CommandCancelAck,
  type CommandCancelRequest,
  type CommandInput,
  type CommandInvocation,
  type CommandInvocationAck,
  type CommandLifecycleEvent,
  type CommandResult,
  type CommandResultAck,
} from './commandBridge';
import { waitForWailsBridge } from './waitForWailsBridge';

interface CommandBridgeAppAPI {
  CommandBridgeInvoke(invocation: CommandInvocation, owner: CommandBridgeOwner): Promise<CommandInvocationAck>;
  CommandBridgeInput(input: CommandInput): Promise<CommandInvocationAck>;
  CommandBridgeAcceptResult(result: CommandResult): Promise<CommandResultAck>;
  CommandBridgeCancel(request: CommandCancelRequest): Promise<CommandCancelAck>;
  CommandBridgeLifecycle(event: CommandLifecycleEvent): Promise<void>;
}

type WailsCommandWindow = Window & {
  go?: {
    app?: {
      App?: Partial<CommandBridgeAppAPI>;
    };
  };
};

export interface CommandBridgeWailsTarget {
  readonly signal?: AbortSignal;
  readonly target?: WailsCommandWindow;
  readonly timeoutMs?: number;
}

function commandBridgeApp(target: WailsCommandWindow): CommandBridgeAppAPI {
  const app = target.go?.app?.App;
  if (
    typeof app?.CommandBridgeInvoke !== 'function' ||
    typeof app.CommandBridgeInput !== 'function' ||
    typeof app.CommandBridgeAcceptResult !== 'function' ||
    typeof app.CommandBridgeCancel !== 'function' ||
    typeof app.CommandBridgeLifecycle !== 'function'
  ) {
    throw new Error('Command bridge Wails API is not available');
  }
  return app as CommandBridgeAppAPI;
}

async function resolveCommandBridgeApp(options: CommandBridgeWailsTarget = {}): Promise<CommandBridgeAppAPI> {
  const target = options.target ?? (window as WailsCommandWindow);
  await waitForWailsBridge({ signal: options.signal, target, timeoutMs: options.timeoutMs });
  return commandBridgeApp(target);
}

export async function dispatchCommandBridgeInvocation(invocation: CommandInvocation, owner: CommandBridgeOwner, options: CommandBridgeWailsTarget = {}): Promise<CommandInvocationAck> {
  return (await resolveCommandBridgeApp(options)).CommandBridgeInvoke(invocation, owner);
}

export async function dispatchCommandBridgeInput(input: CommandInput, options: CommandBridgeWailsTarget = {}): Promise<CommandInvocationAck> {
  return (await resolveCommandBridgeApp(options)).CommandBridgeInput(input);
}

export async function acceptCommandBridgeResult(result: CommandResult, options: CommandBridgeWailsTarget = {}): Promise<CommandResultAck> {
  return (await resolveCommandBridgeApp(options)).CommandBridgeAcceptResult(result);
}

export async function cancelCommandBridgeInvocation(request: CommandCancelRequest, options: CommandBridgeWailsTarget = {}): Promise<CommandCancelAck> {
  return (await resolveCommandBridgeApp(options)).CommandBridgeCancel(request);
}

export async function dispatchCommandBridgeLifecycle(event: CommandLifecycleEvent, options: CommandBridgeWailsTarget = {}): Promise<void> {
  await (await resolveCommandBridgeApp(options)).CommandBridgeLifecycle(event);
}
