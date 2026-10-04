<template>
  <section class="fate-wrap thread">
    <header>
      <h2 class="fate-h2">Three hands on one thread.</h2>
      <p>
        In the myth, three Fates spin, measure and cut the thread of a life. A statechart does the
        same to a process, and fate is the part that does it the same way every time.
      </p>
    </header>
    <ol>
      <li>
        <div class="line"><i class="dot" /><i class="solid" /></div>
        <span class="who">01 / Clotho spins</span>
        <h3>Declare</h3>
        <p>
          Write down every state and transition up front. <code>CreateMachine</code> validates the
          config and returns an immutable machine. Nothing about it changes at runtime.
        </p>
      </li>
      <li>
        <div class="line"><i class="dashed" /></div>
        <span class="who">02 / Lachesis measures</span>
        <h3>Send</h3>
        <p>
          <code>Send</code> dispatches an event and the engine computes the next configuration. No
          I/O, no clock, no randomness. Delays and invocations surface as data for an adapter to
          carry out.
        </p>
      </li>
      <li>
        <div class="line">
          <i class="solid" />
          <svg viewBox="0 0 30 30" aria-hidden="true">
            <circle class="ring" cx="15" cy="15" r="12" />
            <circle class="core" cx="15" cy="15" r="5" />
          </svg>
        </div>
        <span class="who">03 / Atropos cuts</span>
        <h3>Finish</h3>
        <p>
          A final state ends the run. At any point before it, <code>Persist</code> writes the actor
          to JSON and it restores byte for byte, so the thread can be picked up anywhere.
        </p>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.thread {
  display: flex;
  flex-direction: column;
  gap: 56px;
  padding-top: 96px;
  padding-bottom: 96px;
}

header {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 640px;
}

header p {
  line-height: 1.5;
  font-size: 17px;
  color: var(--fate-muted);
}

ol {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
  gap: 40px 0;
}

li {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 40px;
}

.line {
  display: flex;
  align-items: center;
  margin: 0 -40px 10px 0;
  height: 30px;
}

.line i {
  flex: 1 1 auto;
  border-top: 2.4px solid var(--fate-active-line);
}

.line .dashed {
  border-top-style: dashed;
}

.line .dot {
  flex: 0 0 auto;
  border: 0;
  border-radius: 50%;
  width: 12px;
  height: 12px;
  background: var(--fate-active-line);
}

.line svg {
  flex: 0 0 auto;
  width: 30px;
  height: 30px;
}

.ring {
  fill: none;
  stroke: var(--fate-active-line);
  stroke-width: 2.4;
}

.core {
  fill: var(--fate-active-line);
}

.who {
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--fate-muted);
}

h3 {
  line-height: 1.2;
  font-size: 24px;
  font-weight: 600;
}

li p {
  line-height: 1.5;
  font-size: 15px;
  color: var(--fate-muted);
}

code {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--fate-accent);
}
</style>
