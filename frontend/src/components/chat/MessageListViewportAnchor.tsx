import { Component, type ReactNode, type RefObject } from 'react';

interface Props {
  containerRef: RefObject<HTMLDivElement>;
  itemKey: string | null;
  children: ReactNode;
}
interface Snapshot { row: HTMLElement; top: number }

/** O snapshot anterior à mutação é necessário: layout effects já veem o DOM novo. */
export class MessageListViewportAnchor extends Component<Props, object, Snapshot | null> {
  getSnapshotBeforeUpdate(previous: Props): Snapshot | null {
    if (!this.props.itemKey || previous.itemKey !== this.props.itemKey) return null;
    const row = Array.from(this.props.containerRef.current?.querySelectorAll<HTMLElement>('[data-timeline-key]') ?? [])
      .find(element => element.dataset.timelineKey === this.props.itemKey);
    return row ? { row, top: row.getBoundingClientRect().top } : null;
  }

  componentDidUpdate(_previous: Props, _state: object, snapshot: Snapshot | null) {
    const container = this.props.containerRef.current;
    if (!container || !snapshot?.row.isConnected) return;
    container.scrollTop += snapshot.row.getBoundingClientRect().top - snapshot.top;
  }

  render() { return this.props.children; }
}
