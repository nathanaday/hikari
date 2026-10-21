1. What is in this spec?

Yes agreed, especially the system map scehma

2. How does the agent write to your repo?

No, the configuration will define the git branch to work with. Work should be automatically commited to keep this streamlined. However, that the agent may not have the right git credentials is a good point. open question: allow git in the container (commit, branch, merge), or have the agent post a change log that an external agent on the host machine can apply as-is. The working assumption is that the user will not work in the repository at the same time as the agent.

3. How big is the scope?

Agree

4. Where is the map stored and who writes it?

I agree with the location. I think the worker should own it, however. Is it not the case that the orchestrator agent doesn't see or concern itself with the codebase details? so the accurate feature map can only come from the agent that owns the codebase. Please clarify.

5. What does a feature record hold and what does the orchestrator see?

Agree

Sub-questoin (a)

Yes just the test coverage for the entire feature item, it's somewhat based on the honor system of the underlying agent that owns the map, but the agent has nothing to gain by not reporting the test results accurately.

Sub-question (b)

Agree

6. When is the map built, and how does it stay true?

Agree

7. Who can query whose map?

I don't think an agent should read the systems owned by other agents, because then they turn into a bit of an orchestrator themsevles. If one agent has a blocker, they can elevate it to the orchestrator, then the orchestrator can research and find the answer.

On the other points I agree.

8. Can an orchestrator own a system?

No for now (your statement)

9. How does a worker run the tests?

A great point and to be honest, we probably don't know yet. Let's start simple for now - a container has a list of modules/deps it can install so the worker can run its own tests with dependencies.

10. How strict is "the only thing the agent can reach"? 

Yes internet access is ok. 

---

Held off for next round:
- formal work order
- change log
- validation agent

