/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { prefersReducedMotion, SWAP_INTERVAL_MS, SWAP_OUT_MS } from "../motion";

export const CARD_SLOTS = 3;
const PAUSE_RECHECK_MS = 250;

/**
 * Which resource each slot shows. With more resources than slots, one slot at a time (in
 * order) swaps its card for the one that has been hidden longest.
 */
export function useRotatingSlots(total: number) {
  const slotCount = Math.min(CARD_SLOTS, total);
  const [order, setOrder] = useState<number[]>(() => Array.from({ length: total }, (_, i) => i));
  const [turn, setTurn] = useState(0);
  const [leavingSlot, setLeavingSlot] = useState<number | null>(null);
  const [pauseCount, setPauseCount] = useState(0);
  const paused = pauseCount > 0;

  useEffect(() => {
    setOrder(Array.from({ length: total }, (_, i) => i));
    setTurn(0);
  }, [total]);

  // Read inside the timers so pausing never cancels a swap that is already under way.
  const pausedRef = useRef(paused);
  pausedRef.current = paused;

  useEffect(() => {
    if (total <= slotCount || prefersReducedMotion()) return undefined;
    const slot = turn % slotCount;
    let timer: ReturnType<typeof setTimeout>;
    const startSwap = () => {
      if (pausedRef.current) {
        timer = setTimeout(startSwap, PAUSE_RECHECK_MS);
        return;
      }
      setLeavingSlot(slot);
      timer = setTimeout(() => {
        setOrder((current) => {
          const next = [...current];
          [next[slot], next[slotCount]] = [next[slotCount], next[slot]];
          // The card that just left goes to the back of the hidden queue.
          return [...next.slice(0, slotCount), ...next.slice(slotCount + 1), next[slotCount]];
        });
        setLeavingSlot(null);
        setTurn((value) => value + 1);
      }, SWAP_OUT_MS);
    };
    timer = setTimeout(startSwap, SWAP_INTERVAL_MS / 2);
    return () => {
      clearTimeout(timer);
      setLeavingSlot(null);
    };
  }, [total, slotCount, turn]);

  // Hover and focus each add a pause, so one ending does not resume while the other holds.
  const setPaused = useCallback(
    (pause: boolean) => setPauseCount((count) => Math.max(0, count + (pause ? 1 : -1))),
    [],
  );
  // The effect below resets the order when the total changes; until then, use a valid one.
  const validOrder = order.length === total ? order : Array.from({ length: total }, (_, i) => i);

  return { order: validOrder.slice(0, slotCount), leavingSlot, setPaused };
}
